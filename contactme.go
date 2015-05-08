package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
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
	"text/template"
	"time"

	"github.com/rafaeljusto/contactme/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/rafaeljusto/contactme/Godeps/_workspace/src/gopkg.in/yaml.v2"
)

const (
	defaultPort               = 80
	defaultEmailSubjectPrefix = "[ContactMe] "
	defaultEmailTemplate      = `Client: {{.ClientName}}
-------------------------------------
{{.Message}}
-------------------------------------

E-mail sent via ContactMe.
http://github.com/rafaeljusto/contactme`
	defaultLog              = "/var/log/contactme.log"
	defaultRateLimitBurst   = 5.0
	defaultRateLimitRate    = 0.00035
	defaultRateLimitExpires = 25 * time.Hour
	defaultRateLimitCleanup = 5 * time.Minute

	// Possible exit codes on error
	errOpeningConfigFile    = 1
	errReadingConfigFile    = 2
	errSettingPort          = 3
	errParsingMailbox       = 4
	errMissingParameters    = 5
	errReadingEmailTemplate = 6
)

var (
	ratelimit     map[string]map[string]string
	ratelimitLock sync.RWMutex

	undesiredChars = regexp.MustCompile(`(['<>])|\\"|[^\x09\x0A\x0D\x20-\x7E\xA1-\xFF]`)

	config = struct {
		Port       int
		Mailserver struct {
			Address  string
			Username string
			Password string
		}
		Mailbox string
		Email   struct {
			SubjectPrefix string `yaml:"subject prefix"`
			Template      string
		}
		Log       string
		RateLimit struct {
			Burst   float64
			Rate    float64
			Expires time.Duration
			Cleanup time.Duration
		}
	}{
		Port: defaultPort,
		Email: struct {
			SubjectPrefix string `yaml:"subject prefix"`
			Template      string
		}{
			SubjectPrefix: defaultEmailSubjectPrefix,
			Template:      defaultEmailTemplate,
		},
		Log: "/var/log/contactme.log",
		RateLimit: struct {
			Burst   float64
			Rate    float64
			Expires time.Duration
			Cleanup time.Duration
		}{
			Burst:   defaultRateLimitBurst,
			Rate:    defaultRateLimitRate,
			Expires: defaultRateLimitExpires,
			Cleanup: defaultRateLimitCleanup,
		},
	}

	// Parsed template
	emailTemplate *template.Template
)

func init() {
	ratelimit = make(map[string]map[string]string)
}

func main() {
	app := cli.NewApp()
	app.Name = "contactme"
	app.Usage = "E-mail service with HTTP interface"
	app.Author = "Rafael Dantas Justo"
	app.Version = "0.1"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "config,c",
			EnvVar: "CONTACTME_CONFIG",
			Usage:  "Configuration file (other params have priority)",
		},
		cli.StringFlag{
			Name:   "port",
			EnvVar: "CONTACTME_PORT",
			Usage:  "Port to listen to (default: 80)",
		},
		cli.StringFlag{
			Name:   "mailserver,s",
			EnvVar: "CONTACTME_MAILSERVER",
			Usage:  "E-mail server address with port (e.g. smtp.gmail.com:587)",
		},
		cli.StringFlag{
			Name:   "username,u",
			EnvVar: "CONTACTME_USERNAME",
			Usage:  "E-mail server authentication username (default: same of mailbox)",
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
		readCommandLineInputs(c)
		fillConfigurationDefaults()
		validateConfiguration()

		logFile := startLog()
		if logFile != nil {
			defer func() { logFile.Close() }()
		}

		go cleanup()

		http.HandleFunc("/", handle)
		log.Fatal(http.ListenAndServe(":"+strconv.Itoa(config.Port), nil))
	}

	app.Run(os.Args)
}

func readCommandLineInputs(c *cli.Context) {
	if configFile := c.String("config"); configFile != "" {
		data, err := ioutil.ReadFile(configFile)
		if err != nil {
			fmt.Printf("error opening configuration file. Details: %s\n", err)
			os.Exit(errOpeningConfigFile)
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			fmt.Printf("error reading configuration file. Details: %s\n", err)
			os.Exit(errReadingConfigFile)
		}
	}

	if port := c.String("port"); port != "" {
		var err error
		config.Port, err = strconv.Atoi(port)
		if err != nil {
			fmt.Printf("error setting port. Details: %s\n", err)
			os.Exit(errSettingPort)
		}
	}

	if mailserver := c.String("mailserver"); mailserver != "" {
		config.Mailserver.Address = mailserver
	}

	if username := c.String("username"); username != "" {
		config.Mailserver.Username = username
	}

	if password := c.String("password"); password != "" {
		config.Mailserver.Password = password
	}

	if mailbox := c.String("mailbox"); mailbox != "" {
		config.Mailbox = mailbox
	}
}

func fillConfigurationDefaults() {
	if config.Port == 0 {
		config.Port = defaultPort
	}

	config.Mailserver.Username = strings.TrimSpace(config.Mailserver.Username)
	if config.Mailserver.Username == "" {
		config.Mailserver.Username = config.Mailbox
	}

	config.Email.Template = strings.TrimSpace(config.Email.Template)
	if config.Email.Template == "" {
		config.Email.Template = defaultEmailTemplate
	}

	config.Log = strings.TrimSpace(config.Log)
	if config.Log == "" {
		config.Log = defaultLog
	}

	if config.RateLimit.Burst == 0 {
		config.RateLimit.Burst = defaultRateLimitBurst
	}

	if config.RateLimit.Rate == 0 {
		config.RateLimit.Rate = defaultRateLimitRate
	}

	if config.RateLimit.Expires.Seconds() == 0 {
		config.RateLimit.Expires = defaultRateLimitExpires
	}

	if config.RateLimit.Cleanup.Seconds() == 0 {
		config.RateLimit.Cleanup = defaultRateLimitCleanup
	}
}

func validateConfiguration() {
	config.Mailserver.Address = strings.TrimSpace(config.Mailserver.Address)
	config.Mailbox = strings.TrimSpace(config.Mailbox)

	if config.Mailserver.Address == "" || config.Mailbox == "" {
		fmt.Println("missing “mailserver” and/or “mailbox” arguments")
		os.Exit(errMissingParameters)
	}

	if _, err := mail.ParseAddress(config.Mailbox); err != nil {
		fmt.Printf("invalid mailbox “%s”\n", config.Mailbox)
		os.Exit(errParsingMailbox)
	}

	var err error
	emailTemplate, err = template.New("ContactMe E-mail").Parse(config.Email.Template)
	if err != nil {
		fmt.Printf("error reading e-mail template. Details: %s\n", err)
		os.Exit(errReadingEmailTemplate)
	}
}

func startLog() *os.File {
	logFile, err := os.OpenFile(config.Log, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("error opening log file, "+
			"so we will use the standard output instead. Details: %s\n", err)

	} else {
		log.SetOutput(logFile)
	}

	return logFile
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

	from, subject, body, err := readRequestInputs(r)
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

func readRequestInputs(r *http.Request) (from, subject string, body bytes.Buffer, err error) {
	name := normalizeInput(r.FormValue("name"))
	from = normalizeInput(r.FormValue("email"))
	subject = normalizeInput(r.FormValue("subject"))
	message := normalizeInput(r.FormValue("message"))

	err = emailTemplate.Execute(&body, struct {
		ClientName string
		Message    string
	}{
		ClientName: name,
		Message:    message,
	})

	if err != nil {
		return
	}

	_, err = mail.ParseAddress(from)
	return
}

func sendEmail(from, subject string, body bytes.Buffer) error {
	header := map[string]string{
		"From":                      from,
		"To":                        config.Mailbox,
		"Subject":                   config.Email.SubjectPrefix + subject,
		"MIME-Version":              "1.0",
		"Content-Type":              `text/plain; charset="utf-8"`,
		"Content-Transfer-Encoding": "base64",
	}

	var message string
	for key, value := range header {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString(body.Bytes())

	var auth smtp.Auth
	if config.Mailserver.Username != "" && config.Mailserver.Password != "" {
		auth = smtp.PlainAuth("",
			config.Mailserver.Username,
			config.Mailserver.Password,
			config.Mailserver.Address[:strings.Index(config.Mailserver.Address, ":")],
		)
	}

	return smtp.SendMail(
		config.Mailserver.Address,
		auth,
		from,
		[]string{config.Mailbox},
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

	level := config.RateLimit.Burst

	if levelString, ok := ratelimitItem["level"]; ok {
		var err error
		level, err = strconv.ParseFloat(levelString, 64)
		if err != nil {
			return false, err
		}

		diff := now.Sub(lastEvent).Seconds()
		level = math.Min(config.RateLimit.Burst, level+diff*config.RateLimit.Rate)
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

			if now.Sub(lastEvent) <= config.RateLimit.Expires {
				newRatelimit[ip] = ratelimitItem
			}
		}
		ratelimit = newRatelimit
		ratelimitLock.Unlock()

		time.Sleep(config.RateLimit.Cleanup)
	}
}
