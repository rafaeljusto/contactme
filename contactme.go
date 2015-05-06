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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rafaeljusto/contactme/Godeps/_workspace/src/github.com/codegangsta/cli"
)

var (
	mailserver, username, password, mailbox string

	ratelimit     map[string]map[string]string
	ratelimitLock sync.RWMutex

	burst = 5.0
	rate  = 0.00035
)

func init() {
	ratelimit = make(map[string]map[string]string)

	go func() {
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

			time.Sleep(5 * time.Minute)
		}
	}()
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
			Usage:  "Port to listen to",
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
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if granted, err := grant(ip); !granted {
		w.WriteHeader(427)
		return

	} else if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	subject := r.FormValue("subject")
	body := fmt.Sprintf(`Client: %s
-------------------------------------
%s
-------------------------------------

E-mail sent via ContactMe.
http://github.com/rafaeljusto/contactme
		`, name, r.FormValue("message"))

	if _, err := mail.ParseAddress(email); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	header := make(map[string]string)
	header["From"] = email
	header["To"] = mailbox
	header["Subject"] = "[ContactMe] " + subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	var auth smtp.Auth
	if username != "" && password != "" {
		auth = smtp.PlainAuth("", username, password, mailserver[:strings.Index(mailserver, ":")])
	}

	if err := smtp.SendMail(mailserver, auth, email, []string{mailbox}, []byte(message)); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)

	} else {
		w.WriteHeader(http.StatusOK)
	}
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
