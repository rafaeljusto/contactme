# ContactMe

Easy way to send an e-mail for your "contact me" web form.

## Features

* Remove undesireble characters (avoid script attacks)
* Validate client e-mail format
* Limit the number of e-mails from a client in a specific period
* E-mail encoded in base64
* Allow plain authentication with mail server
* Errors and warnings are logged in "/var/log/contactme.log" with fallback for standard output

## API

Expect a POST request containing the fields:

| Field   | Description                          |
| -----   | -----------                          |
| name    | Client name                          |
| email   | Client e-mail to return the contact  |
| subject | Subject of the client                |
| message | Message of the client                |

## Rate Limit

* Use the [token bucket](http://en.wikipedia.org/wiki/Token_bucket) strategy
* Rate limit per IP fixed in 5 e-mails per day (burst)
* Cleanup for entries older than a day (goroutine running every 5 minutes)

## HTTP status

| Status | Description                          |
| ------ | -----------                          |
| 200    | E-mail sent                          |
| 400    | Invalid client e-mail format         |
| 405    | Only POST requests are allowed       |
| 427    | Client already sent too many e-mails |
| 500    | Something went wrong in server-side  |

## Use it

This service has the following parameters to run:

| Parameter  | Environment Varible  | Description                                                      |
| ---------  | -------------------  | -----------                                                      |
| port       | CONTACTME_PORT       | Port to listen to (default: 80)                                  |
| mailserver | CONTACTME_MAILSERVER | E-mail server address with port (e.g. smtp.gmail.com:587)        |
| username   | CONTACTME_PASSWORD   | E-mail server authentication password (default: same of mailbox) |
| mailbox    | CONTACTME_MAILBOX    | E-mail address that will receive all the e-mails                 |

It is recommended before running the service to set the environment variables instead of using the
command line parameters for safety reasons (you don't want your password visible in the process
list).

Command line example (without using environment variables):

```
contactme --port 80 -s smtp.gmail.com:587 -u my@email.com -p "crazypassword" -m my@email.com
```

## E-mail template

```
Client: <client's name>
-------------------------------------
<message>
-------------------------------------
E-mail sent via ContactMe.
http://github.com/rafaeljusto/contactme
```

## Client example

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8"></meta>
    <style>
      input {
        display: block;
        margin-bottom: 20px;
      }

      input[type=submit] {
        margin: 20px auto 0px auto;
      }

      label {
        display: block;
      }
    </style>
  </head>
  <body>
    <form id="contactme">
      <fieldset>
        <legend>Contact Me</legend>

        <label for="name">Name</label>
        <input type="text" id="name" name="name" />

        <label for="email">E-mail</label>
        <input type="email" id="email" name="email" />

        <label for="subject">Subject</label>
        <input type="text" id="subject" name="subject" />

        <label for="message">Message</label>
        <textarea id="message" name="message" rows="10" cols="80"></textarea>
      </fieldset>

      <input type="submit" value="Send" />
    </form>

    <script src="//code.jquery.com/jquery-2.1.4.min.js"></script>
    <script type="text/javascript">
      $(function() {
        $("#contactme").submit(function(e) {
          e.preventDefault();

          var data = $(this).serialize();
          $.post("http://localhost", data)
            .done(function() {
              alert("E-mail sent!");
            })
            .fail(function() {
              alert("Error!");
            });
        });
      });
    </script>
  </body>
</html>
```
