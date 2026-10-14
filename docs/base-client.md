# The base client

Generated code sends every request through the `client` package, after go-azure-sdk's `sdk/client`. It is built on [go-kt](https://github.com/katbyte/go-kt)'s `chttp`: a read is asked again for a dropped connection or a gateway's answer and a write is sent once, an answer is read whole up to a limit, and every exchange is traced to a logger when one is handed in, with the credentials hidden.

## What a call asks for

A call asks for what its operation is documented to answer. One that answers JSON, or nothing, sends `Accept: application/json`. One that answers a file or an event stream asks for that type first and JSON after it, because some servers choose their answer by the header. Where the choice is the caller's, call by call, a workaround gives the operation an `Accept` option.

## What is one service's own

What is common to every service is in the package. What is one service's own about being talked to is a `client.Service`, which the repository writes by hand in a file beside the generated ones and names in its config (`ClientService`). The file has no generated header, so `generate` never writes or removes it.

```go
// sdk/radarr/service.go
package radarr

// service is what is Radarr's own about being talked to.
var service = client.Service{
	Name:           "Radarr",
	UserAgent:      "radarr-mcp/" + version.Version,
	Example:        "nas:7878",
	RedirectAdvice: "set the server url to the address Radarr itself answers on, with its URL base if it has one",
	Messages:       errorMessages, // what Radarr said went wrong, read out of its error body
	Note:           note,          // what a 401 means here, and a 404 that is not JSON
}
```

| A service says | Without it |
|---|---|
| `Name`, `UserAgent`, `Example` | "the server", Go's user agent, no example address beside a refused url |
| `SecretHeaders`, `SecretNames` | `Authorization` and `X-Api-Key` are hidden from a trace, and what chttp hides unasked |
| `FollowRedirects`, `RedirectAdvice`, `WebPageAdvice` | a redirect no operation documents is refused |
| `Timeout`, `HeaderWait`, `MaxResponse` | two minutes for a request, all of it for the server to start answering, 64 MiB held |
| `Retry` | chttp's rule: a read, three times, for a dropped connection or a 502, 503 or 504 |
| `Messages`, `Note` | an error shows the start of what the server sent |
| `SendDateAs` | every date parameter goes out in the service's first layout |
| `Outcome` | a 200 is an answer, whatever its body says |

## New and the credential

The generated `New(baseURL, token, opts...)` wraps the token in the authorizer `Auth` names (`client.APIKey` for a key in `X-Api-Key`, `client.Bearer` for a bearer token) and passes the options on. `client.WithLog`, `WithTransport`, `WithRetry` and `WithCookies` are what the application using the package decides. A service that calls its credential something else says so with `Credential` (`"API key"` makes it `New(baseURL, apiKey, opts...)`), and words New's documentation with `NewDoc`. One that takes its credential in a way of its own writes the authorizer in the same file and gives its config a `Client` source whose `New` uses it.

A value the client must never repeat is kept out of every error it makes: the credential, and any more named with `client.WithSecrets`, read `***` wherever they would appear. A value shorter than six characters is left as it is. The generated `TestNew` proves it on a refusal that echoes the token.

## Redirects

What happens to a redirect is the document's to say, operation by operation:

| The operation documents | A redirect is |
|---|---|
| no redirect | refused with the service's advice, or followed if the service sets `FollowRedirects` |
| a redirect and no 2xx (a login that sends its caller on) | the answer: handed back unfollowed |
| a redirect beside a 2xx | on the way to the answer: followed, the credentials staying on the server's host |

## Errors

An error for an undocumented status is a `*client.StatusError`, with the server's words kept apart as `Messages`. `client.StatusCode` and `Messages` read one out of an error, and `IsNotFound`, `IsUnauthorized`, `IsForbidden`, `IsBadRequest` (400 or 422), `IsConflict`, `IsTooManyRequests` and `IsServerError` say which it is. The types package of a grouped layout offers each under its own name.

A service that answers 200 to everything and says in the body whether the call worked declares `Outcome` in its own file. Such a call returns a `*client.OutcomeError` with the answer still decoded, and `client.OutcomeValue(err)` is the value to branch on.

## Calls

A JSON-RPC package has no base client. Its `New` takes a `client.Caller`, which the repository writes over its connection: `Call(ctx, method, params, into)` sends the method and its arguments and decodes the answer. A refusal is a `*client.CallError` carrying the server's code, message, error number and name, reason and whatever more it attached. `client.CallErrorOf` reads it out of an error, `client.IsRefusedCall` tells a refusal from a connection that failed, and `client.CallParams` trims the arguments a method leaves off the end.

A method whose answer is a file, or that takes files beside its arguments, needs more of the connection: `client.Downloader` runs the method and writes the file to a writer, and `client.Uploader` sends the files with the call and decodes what the method answers. How the file travels is the connection's own. A connection that offers neither fails such a call with `client.ErrCannotCarryFiles`.

## A value the server hides

A server that shows a secret only to some callers sends the rest a placeholder, `********`, in its place whatever the field's type, so a number or a record arrives as text and the whole answer fails to read. A field the `NamedTypes` workaround holds as `client.Secret[T]` reads either: `Value` is the value and `Hidden` says the placeholder came. Written back, a hidden value goes out as the placeholder it came as. Printed, by itself or inside a record, it shows `***`, so a record logged by mistake gives nothing away.

## A base client of the repository's own

A repository can keep a base client of its own and name it in `ClientImport`. Generated code calls `New(baseURL, authorizer, ...Option)`, `RequestOptions`, `NewRequest`, `Marshal`, `SetBody`, `Execute`, `Response.Unmarshal`, `Headers` and `QueryParams` with `Append`, `CSV`, `JSONObject`, `DeepObject`, `DefaultPageSize` and `StatusCode`. Its tests set `HTTPClient` and read `BaseURL`.
