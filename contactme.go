package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rafaeljusto/contactme/Godeps/_workspace/src/github.com/codegangsta/cli"
)

const (
	burst        = 5.0
	rate         = 0.00035
	cleanupSleep = 5 // minutes

	subjectPrefix = "[ContactMe] "
	template      = `Client: %s
-------------------------------------
%s
-------------------------------------

E-mail sent via ContactMe.
http://github.com/rafaeljusto/contactme`
)

var (
	mailserver, username, password, mailbox string

	ratelimit     map[string]map[string]string
	ratelimitLock sync.RWMutex

	undesiredChars = regexp.MustCompile(`(['<>])|\\"|[^\x09\x0A\x0D\x20-\x7E\xA1-\xFF]`)
)

func init() {
	ratelimit = make(map[string]map[string]string)
	go cleanup()
}

func main() {
	app := cli.NewApp()
	app.Name = "contactme"
	app.Usage = "E-mail service with HTTP interface"
	app.Author = "Rafael Dantas Justo"
	app.Version = "0.1"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "port",
			EnvVar: "CONTACTME_PORT",
			Usage:  "Port to listen to (default 80)",
		},
		cli.StringFlag{
			Name:   "mailserver,s",
			EnvVar: "CONTACTME_MAILSERVER",
			Usage:  "E-mail server address with port",
		},
		cli.StringFlag{
			Name:   "username,u",
			EnvVar: "CONTACTME_USERNAME",
			Usage:  "E-mail server authentication username",
		},
		cli.StringFlag{
			Name:   "password,p",
			EnvVar: "CONTACTME_PASSWORD",
			Usage:  "E-mail server authentication password",
		},
		cli.StringFlag{
			Name:   "mailbox,m",
			EnvVar: "CONTACTME_MAILBOX",
			Usage:  "E-mail address that will receive all the e-mails",
		},
	}

	app.Action = func(c *cli.Context) {
		port := c.String("port")
		mailserver = c.String("mailserver")
		username = c.String("username")
		password = c.String("password")
		mailbox = c.String("mailbox")

		if port == "" {
			port = "80"
		}

		if mailserver == "" || mailbox == "" {
			fmt.Println("Missing “mailserver” and/or “mailbox” arguments\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		if _, err := mail.ParseAddress(mailbox); err != nil {
			fmt.Println("Invalid “mailbox”!\n")
			cli.ShowAppHelp(c)
			os.Exit(2)
		}

		http.HandleFunc("/", handle)
		log.Fatal(http.ListenAndServe(":"+port, nil))
	}

	app.Run(os.Args)
}

func handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Println("invalid remote address. Details:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if granted, err := grant(ip); !granted {
		w.WriteHeader(427)
		return

	} else if err != nil {
		log.Println("error in rate limit. Details:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	from, subject, body, err := readInputs(r)
	if err != nil {
		log.Printf("invalid input from “%s”. Details: %s", ip, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := sendEmail(from, subject, body); err != nil {
		log.Println("error sending e-mail. Details:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func readInputs(r *http.Request) (from, subject, body string, err error) {
	name := normalizeInput(r.FormValue("name"))
	from = normalizeInput(r.FormValue("email"))
	subject = normalizeInput(r.FormValue("subject"))
	body = fmt.Sprintf(template, name, normalizeInput(r.FormValue("message")))

	_, err = mail.ParseAddress(from)
	return
}

func sendEmail(from, subject, body string) error {
	header := map[string]string{
		"From":                      from,
		"To":                        mailbox,
		"Subject":                   subjectPrefix + subject,
		"MIME-Version":              "1.0",
		"Content-Type":              `text/plain; charset="utf-8"`,
		"Content-Transfer-Encoding": "base64",
	}

	var message string
	for key, value := range header {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	var auth smtp.Auth
	if username != "" && password != "" {
		auth = smtp.PlainAuth("", username, password, mailserver[:strings.Index(mailserver, ":")])
	}

	return smtp.SendMail(
		mailserver,
		auth,
		from,
		[]string{mailbox},
		[]byte(message),
	)
}

func normalizeInput(input string) string {
	input = strings.TrimSpace(input)
	input = undesiredChars.ReplaceAllString(input, "")
	return input
}

func grant(ip string) (bool, error) {
	ratelimitLock.RLock()
	ratelimitItem := ratelimit[ip]
	ratelimitLock.RUnlock()

	now := time.Now().UTC()
	lastEvent := now

	if last, ok := ratelimitItem["last"]; ok {
		var err error
		lastEvent, err = time.Parse(time.RFC3339Nano, last)
		if err != nil {
			return false, err
		}
	}

	level := burst

	if levelString, ok := ratelimitItem["level"]; ok {
		var err error
		level, err = strconv.ParseFloat(levelString, 64)
		if err != nil {
			return false, err
		}

		diff := now.Sub(lastEvent).Seconds()
		level = math.Min(burst, level+diff*rate)
	}

	answer := false

	if level >= 1.0 {
		level -= 1.0
		answer = true
	}

	if ratelimitItem == nil {
		ratelimitItem = make(map[string]string)
	}

	ratelimitItem["last"] = now.UTC().Format(time.RFC3339Nano)
	ratelimitItem["level"] = fmt.Sprintf("%f", level)

	ratelimitLock.Lock()
	ratelimit[ip] = ratelimitItem
	ratelimitLock.Unlock()

	return answer, nil
}

func cleanup() {
	for {
		newRatelimit := make(map[string]map[string]string)
		now := time.Now()

		ratelimitLock.Lock()
		for ip, ratelimitItem := range ratelimit {
			last, ok := ratelimitItem["last"]
			if !ok {
				newRatelimit[ip] = ratelimitItem
				continue
			}

			lastEvent, err := time.Parse(time.RFC3339Nano, last)
			if err != nil {
				newRatelimit[ip] = ratelimitItem
				continue
			}

			if now.Sub(lastEvent) <= time.Duration(24)*time.Hour {
				newRatelimit[ip] = ratelimitItem
			}
		}
		ratelimit = newRatelimit
		ratelimitLock.Unlock()

		time.Sleep(cleanupSleep * time.Minute)
	}
}
