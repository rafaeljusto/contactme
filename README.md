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

* Rate limit per IP fixed in 5 e-mails per day (burst)
* Cleanup for entries older than a day (goroutine running every 5 minutes)

## HTTP status

| Status | Description                          |
| ------ | -----------                          |
| 200    | E-mail sent                          |
| 405    | Only POST requests are allowed       |
| 427    | Client already sent too many e-mails |
| 500    | Something went wrong in server-side  |