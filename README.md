# ContactMe

Easy way to send an e-mail for your "contact me" web form.

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
