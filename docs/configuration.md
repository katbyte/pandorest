# Configuring a service

Everything about a service is in its `config.Service`. Only the first four fields are required. Each field's doc comment in `config/config.go` is the reference; this page is the same material with examples.

## Documents and versions

Documents are named by version. `<dir>/<service>-openapi-<version>.json` imports into `<dir>/<service>-<version>/` beside it. The highest version present is the one imported and generated from.

The version is the release the document was taken from. For Emby and Jellyfin that is the document's own `info.version`. For Sonarr, Radarr and Prowlarr, whose documents say `3.0.0` or `1.0.0` whatever the release, it is not, so name the file by the release. The generated package declares both: `APIVersion` is the document's own and `DocumentVersion` the file's.

A service can generate a package for every version it vendors by saying where each goes in `Outputs`. Each package is independent, so a caller can be on one version and import the next beside it for what it needs. `-version` runs a command on one of them.

A JSON-RPC service (`Document: config.JSONRPCDocument`) vendors `<service>-methods-<version>.json` instead: the server's own list of its methods, as TrueNAS answers `core.get_methods`.

To refresh a document, vendor the new one beside the old, import it into its own definitions directory, read `diff -old <old dir> -new <new dir>`, and delete the old pair once the new one is in.

A repository that vendors its dependencies takes a new pandorest in this order: `go get` it, regenerate with `-mod=mod` (`go run -mod=mod ./sdk/pandorest generate`), then `go mod tidy` and `go mod vendor`. The other way round, the stale `vendor/` and the generated files' old imports each stop the other from building.

## The fields

| Field | Says |
|---|---|
| `Name` | the `-service` name, and the prefix of the document and definitions names |
| `Package`, `Output` | the generated package's name and directory, for a service generated from its highest version alone |
| `Outputs` | each document's version to its package directory (`"4.10.0.40": "sdk/emby/4.10.0.40"`). Every version listed must be vendored and every document listed. `Output` is then unused |
| `Document` | an OpenAPI document (the default) or `config.JSONRPCDocument`. For JSON-RPC each method is a call with its arguments in order, grouped by its namespace, and the generated `New` takes a `client.Caller` the repository writes over its connection. `Auth` then says nothing |
| `ReadRole`, `Reads`, `NotReads` | for JSON-RPC: the role a method must allow to count as a read (TrueNAS's `READONLY_ADMIN`), the methods that are reads without it, and those that are not with it. Each name is held to the document |
| `TypedUnions` | a value that is one of several records becomes a type of its own that hands the record over as the member asked for, told apart by the field the records name themselves by; off, it is raw JSON |
| `Grouped`, `Packages`, `Module`, `ClientPackage` | the grouped layout (see [generated code](generated-code.md)): a package per tag, named for the tag unless `Packages` says otherwise (`"Package": "packages"`). The packages import one another under `Module`, read from `go.mod` when unset |
| `Auth` | the name of the base client's authorizer. The generated `New(baseURL, token)` calls `client.<Auth>(token)`: `APIKey` or `Bearer` on pandorest's own |
| `ClientService` | the name of a `client.Service` written by hand in the generated package, which the generated `New` makes its client from |
| `ClientImport` | the import path of a base client of the repository's own, in place of pandorest's |
| `Dir` | where the documents and definitions live; `api-defs` unless set |
| `Naming` | `PathNaming` names a method after its HTTP method and path; `OperationIDNaming` after its operationId |
| `PathPrefix`, `Words` | for path naming: a prefix left out of names (`/api/v3`), and the spelling of run-together segments (`episodefile` as `EpisodeFile`) |
| `TagSuffix` | trimmed from tags to make group names (`Service` on Emby) |
| `Paging`, `Pagings` | the query parameters and answer properties that make an operation a list, and whether it pages by start index or page number. A list gets `Complete` and `Each`. `Limit` may be empty where the server fixes the page size (TMDB), and `Items` where each answer names its list differently (Dockhand). `Pagings` are further shapes for a service whose lists do not all page alike. A service whose pages count from 0 says `FirstPage: new(0)` beside `ByPage` |
| `KeepNull` | fields, as `Schema.property`, held as pointers because their zero is not their absence: a number whose null is not 0 (season 0 is the specials), or a string the server clears only when sent empty |
| `WrittenWhole` | settings schemas read and written back whole, whose empty strings and zeros are sent rather than left out |
| `SendRequired` | a property the document requires is always sent |
| `PreferJSON` | a response that lists JSON beside another type answers JSON, not a file |
| `TextAnswers` | an answer labelled `text/plain` is short text read whole into a string and closed, not a file left open for the caller (Audiobookshelf answers `OK` to most writes). One declared `format: binary`, or by the `File` workaround, stays a file |
| `ExpectSameShape` | a status outside 2xx that answers the success shape is expected, not an error |
| `Dates`, `Days` | how the server writes a date-time and a date. `Send` is the layout a date goes out in (`config.Dates{Send: "2006-01-02T15:04:05.0000000Z07:00"}`), `Read` any others it is read in. Set, every such field and parameter is the package's `Time` or `Day`, left out when zero. A date no layout reads fails the decode naming it, or with `KeepUnread` is read as zero with its text kept. `client.DateObjectMillis` (`"$date"`) is the layout of a date written as `{"$date": milliseconds}`. A parameter wanting another layout is named in the service's own file with `SendDateAs` |
| `IDs`, `Plain` | the kinds of id the package tells apart, each a named type over int or string (`type SeriesID int`), so the compiler refuses a series id where an episode id goes. See [ids](#ids) |
| `InlineSingleOptions` | an operation with one query or header parameter takes it as an argument in place of the options struct |
| `Credential`, `NewDoc` | the generated `New` in the service's words: what its credential is called (`API key` names the parameter `apiKey` and words the refusal of an empty one), and its documentation |
| `Client` | the source of `client.go` in place of the default, for a service that wants a `New` of another shape. It declares no imports and may use the standard packages generated code does |
| `Notes` | a paragraph for the generated package's documentation |

A name the config gets wrong fails the import rather than doing nothing: a `KeepNull` field that is neither a string nor a nullable number, a `WrittenWhole` schema that is not there, a `Words` entry that is not its key's letters.

## Ids

A kind names the models it is the id of (`Of`). From those its base type and every bare path `{id}` are inferred: on any method of a path whose operations answer or take such a model, and on the paths under it. `Names` are the parameters and fields that always mean the kind. `In` names the places inference cannot settle, `Except` the places of a name that are something else there, and `Joined` the parameters that take several as one comma-joined string. A foreign id is a kind by `Base`.

`Shape` is a regular expression every value of the kind matches over its text (Emby's `^[0-9a-f]{32}$`). A method refuses a path parameter or an option without it before any request is sent, naming the value, where a server might answer a malformed id with a bare 500. `Example` is a value of the shape the generated tests send, required with it.

`Plain` names the id-looking parameters and fields left as they are on purpose. The import logs what each kind reached and the names no kind or `Plain` covers, which is the list the setting is written from. `importer.VerifyIDs` is the test a repository's file passes.

`int(id)` and `SeriesID(n)` convert one value, `client.IDs` and `client.StringIDs` a list.
