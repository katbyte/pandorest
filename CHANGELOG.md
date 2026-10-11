## Unreleased

- the grouped layout (`Grouped: true`), after go-azure-sdk's service/version/resource: the package at `Output` holds every model, enum, options and response type and the base client's options and error helpers under the SDK's own name, each tag's operations are a package under it with a `Client` of their own, and `client` holds `New` and a `Client` with a field per tag; `groups.json` beside them maps each operation to its tag for a caller moving over. A caller imports the types package and `client` and never a tag's, so a tag's name may be a word the caller uses for a variable; one that is a Go keyword, a standard library package, `client` or the SDK's name is refused until `Packages` names it. Under a version directory the types package is `<version>/<package>`, so every package is named for its directory. The packages import one another under the repository's module path, from `go.mod` or `Module`. `check` and the sweep find a method through the tag's field. Flat stays the default and nothing moves for a service that does not opt in
- a `Credential` of more than two words ("TMDB read access token or API key") names the parameter `credential` and still words the refusal, where it used to be run together into the parameter's name
- `generate` refuses to overwrite a hand-written Go file at a name it writes to (a `doc.go` moved beside the types), where it used to replace it without a word
- the options struct is `<Name>Options`, no longer `<Name>OperationOptions`: a caller renames by one replacement (`OperationOptions` to `Options`). The response stays `<Name>OperationResponse`, because an operation whose document answers an unnamed object already has its model named `<Name>Response`, as TMDB's and Dockhand's mostly do
- `InlineSingleOptions`, off by default: an operation with one query or header parameter takes it as an argument in place of the options struct, variadic when optional (`c.GetMovieLookup(ctx, term)` or `c.GetMovieLookup(ctx)`; a false or 0 given is sent) and plain when required, a list as its elements; an operation with more keeps the struct, as does a list that pages, and the sweep's by-name fixtures fill the argument. The six sessions asked for one and no more: above one, same-typed neighbours swap silently and a zero can no longer stand for "not sent"
- a method whose options the document all marks optional takes the struct variadically, so a call with no filters is `c.GetSessions(ctx)` and `c.GetItemsComplete(ctx)`; two structs are refused before any request. A call passing one struct is unchanged, so no caller moves; the generated signatures do, in both layouts
- `Auth` may name an authorizer declared in the service's own file, by its lowercase name, where an uppercase one is the base client's; a repository that kept a `Client` source only to call its own authorizer can drop it
- `client.Logger` and `client.Retry` name what `WithLog` and `WithRetry` take

- a service can generate an SDK for every version of its document it vendors, each a package of its own: `Outputs` maps a document's version to its package directory (`"4.10.0.40": "sdk/emby/v1"`), and `import`, `generate`, `check` and `diff` run per version, with `-version` to pick one. The packages are independent, so a caller can import two side by side. A service without `Outputs` is generated from its highest version alone, as before

## v0.6.1 (2026-10-10)

- `KeepNull` takes a string field as well as a nullable number: the field is held as a `*string`, left out when nil and sent as `""` when it points at one, for a server that keeps whatever a write leaves out and clears a field only when it is sent empty (Dockhand's `publicIp`). A string held by value is left out when empty, so such a field could not be cleared
- the README lists the SDKs generated with pandorest, with their import paths and repositories

## v0.6.0 (2026-10-10)

- add ready-made workarounds to `importer/workarounds`, for the bugs most documents have and each repository patched by hand: `UndeclaredAnswers` (a success with no content), `WrongAnswers` (one answer declared, another given), `WrongStatus` (a create documented as 200 that answers 201), `UndeclaredParameters`, `UndeclaredProperties` (fields the server sends that a schema leaves out) and `NotAPI` (the web interface's paths). A repository gives an `About` (name, service, bug) and says which operations; the patching and the check that the bug is still in the document are here. An answer is said with `JSON`, `JSONString`, `Model`, `ListOf`, `Shape`, `File` or `Text`
- `workarounds.Verify` says a workaround "does not apply to the vendored document, so the bug is not there or the workaround is written wrong", where it said only that the bug was not there: a schema name misspelt in a workaround's own table read as the bug having gone
- add text as a kind of answer: short text that is not JSON (a path, a version) is read whole into the method's `Model`, a `*string`, as it was sent and whatever the server calls it, where it was a file its caller had to read and close. Only a workaround asks for it (`workarounds.Text`), since a log is text too; nothing moves for an operation that does not. A base client of a repository's own needs `Response.Text` if it has one
- `openapi.JSONMedia` and `openapi.PickJSON` are the importer's rules for which media types are JSON, for a workaround to share

## v0.5.0 (2026-10-10)

- a workaround can give an operation an `Accept` option, for a server that chooses its answer by the header where the choice is the caller's to make call by call (Dockhand's slow actions wait and answer how it went when asked for JSON alone, and answer at once with a job to follow when asked for anything else). The importer keeps a header parameter named `Accept` that a workaround adds (`AddParameters` with `Param{Name: "Accept", In: openapi.InHeader}`), where it ignores a document's own as OpenAPI says to; the option is generated like any header option, and when set its value is the whole header. Nothing moves for a repository that adds none

## v0.4.0 (2026-10-10)

- a call asks for what its operation is documented to answer: one that answers a file or an event stream sends that type first in `Accept` with JSON after it (`text/event-stream, application/json;q=0.9`), where every call asked for `application/json` alone. A server that chooses its answer by the header (Dockhand's dashboard stream answers JSON to a caller that asks for JSON alone) now sends what its document says. Calls that answer JSON, or nothing, are unchanged
- generator (breaking for a base client of a repository's own): a method that answers a file sets `Accept` in its `RequestOptions`, so such a client needs the field; a regenerated package gains one line in each such method and, in its test, the check of the header (three lines, and the `strings` import where the test had none)
- sweep: an event stream (`text/event-stream`, by its document or by what the server answers) is read as far as its first event and then hung up on, where it was read like a file and ran out of time, so that each one needed a `Skip`. One that sends no event in `EventWait` (ten seconds unless set), or ends without one, is an answer with nothing in it and takes an `Empty` or `MayBeEmpty` case like an empty list; one that answers another content type than the event stream its document says, or breaks off part way, fails

## v0.3.2 (2026-10-10)

- sweep: a path parameter's fixture is looked up by as many of the literal segments before it as there are, the most of them first, where it was only ever the one: `config/indexer/id` is found before `indexer/id`, so the settings under `/config/indexer/{id}` can be given another id than the indexers at `/indexer/{id}` with no case for it. A fixture keyed by one segment or by the name alone is found as before

## v0.3.1 (2026-10-10)

- a service words the generated `New` without copying it: `Credential` is what it calls its credential ("API key" names the parameter `apiKey` and refuses an empty one with "API key is required"), and `NewDoc` is New's documentation; a `Client` source is now only for a `New` of another shape
- the generated `doc.go` no longer starts with the generated header, so that a linter sees the package comment in it: a file written by hand beside the generated ones (the service's own) was otherwise told its package has none, and every repository carried an exception for it. `generator.Generated` still knows `doc.go` as generated; a test that tells generated files by `generator.Header` must use it instead
- `client.Service.Example` can be a host alone (`nas:7878`, shown as `http://nas:7878`), so a service's file holds no `http://` address for a linter to object to

## v0.3.0 (2026-10-10)

- add `client`: the base client every generated package sends its requests through, one copy in place of the one each repository kept, built on go-kt's `chttp`; what is a service's own (its name, how long it may take, what its refusals mean, how its error body reads) is a `client.Service` the repository writes by hand beside its generated package and names with `ClientService`
- generator (breaking): the default `client.go` is built on `client` unless `ClientImport` names another, and its `New` takes the base client's options after the token, so a base client of a repository's own needs an `Option` type and a `New` that takes them
- a status error keeps what the server said as a list (`StatusError.Messages`, `client.Messages`) as well as joined in its text
- what happens to a redirect is the document's to say, per operation: one that documents a redirect and no 2xx answers the redirect, handed back unfollowed (it used to fail the import); one that documents a redirect beside a 2xx expects both and has the redirect followed to its answer; a redirect no operation documents is refused or followed as the service says
- `internal/client`, the stand-in the tests built generated packages against, is gone: they build against `client`

## v0.2.0 (2026-10-10)

- add `sweep`: the read sweep a repository runs against a real server, every GET in a service's definitions called on the generated client and its answer held to decoding, carrying something and losing no object whole; one package made of the copies in embyfin-mcp, sonarr-mcp, radarr-mcp and prowlarr-mcp, with radarr's check for undeclared fields as `Strict` and prowlarr's `Sometimes`
- importer (breaking): two operations that make the same method name fail the import, where the second used to be numbered (`GetAudit2`) with a warning; which one took the number followed the order of the paths, so a refreshed document could rename a method
- a workaround can name an operation (`Name` on `openapi.Operation`), which is how such a clash is settled

## v0.1.0 (2026-10-09)

- initial release: the SDK generator extracted from embyfin-mcp's `sdk/pandorest`, where it writes the Emby, Jellyfin and TMDB SDKs, so the copies in sonarr-mcp, radarr-mcp and prowlarr-mcp can be replaced by one
- a repository hands its services and its workarounds to `pandorest.Main` from a main of a few lines; the service list, the workarounds for a document's bugs and the base client stay in the repository that owns the documents
- the base client a generated package is built on is named per service (`ClientImport`, `Auth`), and a service can give its own `client.go` (`Client`) and a paragraph for the package documentation (`Notes`)
- which operations are lists is said per service (`Paging`): the two parameters and two properties, and whether the list pages by start index (Emby, Jellyfin) or by page number (Sonarr, Radarr, Prowlarr)
- taken from the Sonarr, Radarr and Prowlarr copies, each off unless a service asks: `PathPrefix` and `Words` for method names, `WrittenWhole` for settings saved whole, `SendRequired` for required properties, `PreferJSON` for a response that lists JSON beside another type, and `ExpectSameShape` for a failure that answers the success shape
- a generated package declares `DocumentVersion`, the version its document's file is named by, beside `APIVersion`, the document's own, which for some servers does not move from release to release
- `workarounds.Verify` is the test every workaround must pass, in one call: it applies to its vendored document, fails the second time, and does not panic on a document that has lost its 200 responses
- generated files start `// Code generated by pandorest; DO NOT EDIT.`; a file with the header of one of the earlier copies (`sdk/pandorest`, `internal/pandorest`) is still recognised as generated, so stale ones are removed on the first run
- a service with no lists gets no paged test server in its generated tests and no word of paging in its package documentation
