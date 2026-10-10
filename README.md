# pandorest

[![GitHub release](https://img.shields.io/github/v/release/katbyte/pandorest?color=blueviolet)](https://github.com/katbyte/pandorest/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/katbyte/pandorest?color=00ADD8)](https://github.com/katbyte/pandorest/blob/main/go.mod)
[![License](https://img.shields.io/github/license/katbyte/pandorest?color=blue)](https://github.com/katbyte/pandorest/blob/main/LICENSE)
![build](https://github.com/katbyte/pandorest/actions/workflows/build.yaml/badge.svg)
![test](https://github.com/katbyte/pandorest/actions/workflows/pr-tests.yaml/badge.svg)
![lint](https://github.com/katbyte/pandorest/actions/workflows/pr-golangci-lint.yaml/badge.svg)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/katbyte/pandorest/badges/coverage.json)](https://github.com/katbyte/pandorest/actions/workflows/coverage.yaml)

pandorest generates typed Go SDKs from vendored OpenAPI documents. It was written in embyfin-mcp for the Emby, Jellyfin and TMDB SDKs, copied into three more repositories, and is now here so they share one.

Inspired by [Pandora](https://github.com/hashicorp/pandora), the [go-azure-sdk](https://github.com/hashicorp/go-azure-sdk) generator: the same importer, definitions, differ and generator pipeline, scaled down to a handful of documents in one repository:

```
api-defs/
  emby-openapi-4.10.0.40.json ──┐               ┌─ emby-4.10.0.40/*.json ──┐
  jellyfin-openapi-12.1.0.json ─┼─ import ──────┼─ jellyfin-12.1.0/*.json ─┼─ generate ─ sdk/emby, sdk/jf, sdk/tmdb
  tmdb-openapi-2026.09.22.json ─┘ (workarounds) └─ tmdb-2026.09.22/*.json ─┘
                                                        │
                                         diff ──────────┘  (what a refreshed spec changes)
```

It does what the alternatives were tried and rejected for (oapi-codegen, openapi-generator): errors for undocumented statuses rather than a result per content type, plain string ids, readable names for machine-made operationIds, no builder chains or nullable wrappers, and a test for every operation it writes.

## Using it

pandorest is a library. A repository that generates an SDK keeps what is its own, the documents, the services they describe, the workarounds for their bugs and the base client its generated packages send requests through, and a main of a few lines that hands them over:

```go
// sdk/pandorest/main.go
package main

import (
	"github.com/katbyte/pandorest"
	"github.com/katbyte/pandorest/config"

	"github.com/katbyte/sonarr-mcp/sdk/pandorest/workarounds"
)

func main() {
	pandorest.Main(pandorest.Config{
		Services: []config.Service{{
			Name:         "sonarr",
			Package:      "sonarr",
			Output:       "sdk/sonarr",
			Naming:       config.PathNaming,
			PathPrefix:   "/api/v3",
			ClientImport: "github.com/katbyte/sonarr-mcp/sdk/client",
			Auth:         "APIKey",
			Paging:       &config.Paging{Start: "page", Limit: "pageSize", Items: "records", Total: "totalRecords", ByPage: true},
		}},
		Workarounds: workarounds.All, // the repository's own fixes for its document
	})
}
```

It is run from the repository root, usually by make:

```bash
go run ./sdk/pandorest import      # each service's highest-versioned spec into its definitions, applying the workarounds
go run ./sdk/pandorest generate    # the packages, from those definitions
go run ./sdk/pandorest diff        # what the specs change against the checked-in definitions, breaking changes marked
go run ./sdk/pandorest check       # the definitions match the specs, and every operation has a method
go run ./sdk/pandorest resolve     # which document and definitions each service is at
```

`-service a,b` limits a command to some services, `-root` runs it against another checkout, and `diff -old <dir> -new <dir>` compares two definitions directories. `pandorest.Run` is the same thing as a function, for a test that wants to hold the repository to its own documents.

**Documents are named by version.** `<dir>/<service>-openapi-<version>.json` imports into `<dir>/<service>-<version>/` beside it, and the highest version present is the one imported and generated from. The version is the release the document was taken from: for Emby and Jellyfin that is the document's own `info.version`, and for Sonarr, Radarr and Prowlarr, whose documents say `3.0.0` or `1.0.0` whatever the release, it is not, so name the file by the release. Nothing compares the two. The generated package declares both: `APIVersion` is the document's own and `DocumentVersion` the file's.

To refresh a spec, vendor the new document beside the old one, import it into its own definitions directory, read `diff -old <old dir> -new <new dir>`, and delete the old pair once the new one is in.

## A service

Everything about a service is in its `config.Service`, and everything but the first five is optional.

| Field | Says |
|---|---|
| `Name` | the `-service` name, and the prefix of the document and definitions names |
| `Package`, `Output` | the generated package's name and directory |
| `ClientImport` | the import path of the hand-written base client the package is built on |
| `Auth` | the name of the base client's authorizer: the generated `New(baseURL, token)` calls `client.<Auth>(token)` |
| `Dir` | where the documents and definitions live, `api-defs` unless set |
| `Naming` | `PathNaming` names a method after its HTTP method and path, `OperationIDNaming` after its operationId |
| `PathPrefix`, `Words` | for path naming: a prefix left out of names (`/api/v3`), and the spelling of run-together segments (`episodefile` as `EpisodeFile`) |
| `TagSuffix` | trimmed from tags to make group names (`Service` on Emby) |
| `Paging` | the two query parameters and two answer properties that make an operation a list, and whether it pages by start index or by page number; a list gets a `Complete` method |
| `KeepNull` | number fields, as `Schema.property`, held as pointers because their null is not their zero (season 0 is the specials) |
| `WrittenWhole` | settings schemas read and written back whole, whose empty strings and zeros are sent rather than left out |
| `SendRequired` | a property the document requires is always sent |
| `PreferJSON` | a response that lists JSON beside another type answers JSON, not a file |
| `ExpectSameShape` | a status outside 2xx that answers the success shape is expected, not an error |
| `Client` | the source of `client.go` in place of the default, for a service that wants its own `New` or its own wording; it declares no imports and may use the standard packages generated code does (`errors`, `fmt`, `strings`, `strconv`, `net/http`, `net/url`, `io`, `encoding/json`, `context`, `slices`) |
| `Notes` | a paragraph for the generated package's documentation |

A name the config gets wrong fails the import rather than doing nothing: a `KeepNull` field that is not a nullable number, a `WrittenWhole` schema that is not there, a `Words` entry that is not its key's letters.

## Pieces

| Package | Pandora's | Does |
|---|---|---|
| `pandorest` | the commands | `Main` and `Run`: import, generate, diff, check, resolve |
| `config` | `config/resource-manager.hcl` | a service: its documents, paths, naming and the options above |
| `openapi` | the swagger parser | decodes the subset of OpenAPI 3 the documents use, mutably |
| `importer/workarounds` | `importer-rest-api-specs/components/dataworkarounds` | the interface a repository's workarounds implement, their shared helpers and the test they must pass |
| `importer` | `importer-rest-api-specs` | normalises a patched document into definitions, strictly |
| `definitions` | `api-defs/` + its models | the contract: JSON per version per tag, load, save, validate |
| `differ` | `data-api-differ` | reports operations, options, bodies, models, fields and enum values added, removed or changed |
| `generator` | `generator-go-sdk` | writes the package from definitions only |
| the base client (yours) | `go-azure-sdk/sdk/client` | the hand-written client a repository's generated packages share |

## Importing

`import` loads each document, applies its workarounds, and normalises it:

- **Names.** With `OperationIDNaming` a method is named after its operationId (`GetItems`; TMDB's `movie-details` is `MovieDetails`). With `PathNaming`, for operationIds that are machine-made, lossy or missing, it is named after the method and path: `GET /Items/{Id}/Similar` is `GetItemsByIdSimilar`. Schema names lose their dots and underscores (`QueryResult_BaseItemDto` is `QueryResultBaseItemDto`); fields keep the API's spelling (`Id`, `ImdbId`), and a field with a leading underscore beside its plain twin takes an `Underscore` prefix.
- **Types.** `Integer` (int), `Integer64`, `Float`, `Double`, `String`, `Boolean`, `List`, `Dictionary`, `Reference` to a model or enum, `RawObject` for JSON of no declared shape (untyped objects, unions, "binary" JSON documents), `Any`, and `RawFile` for bodies that are not JSON. An inline object becomes a model named after its owner and field; an operation's inline request and response are named after the operation.
- **Operations.** Path parameters in template order; query and header parameters as options, with lists comma-separated or one key per value as the document says, an object sent as one JSON string or, in `deepObject` style, as `name[key]=value` per key, and a header named `Accept`, `Content-Type` or `Authorization` ignored, as OpenAPI says; the JSON request body, a model whether declared or inline, or raw bytes; the success response (JSON, a file, or nothing); `ExpectedStatusCodes` from the 2xx responses; and `Pageable` for a list.
- **Grouping.** One definitions file per spec tag, holding the tag's operations and the models and enums only its operations use. Anything more than one tag uses is in `Common.json`. There is one Go package per service, not per tag, because shared models would otherwise tie every package to every other.

The importer is strict. An undeclared path parameter, a GET that does not say what it answers, a duplicate operationId, an operation without a tag or a success response, or two schemas that would declare the same Go type fail the import with every problem listed, rather than being guessed at.

### Workarounds

A document bug is fixed with a workaround, kept in the repository that vendors the document: a type with a `Name` (`emby-search-term`), the `Service` it is for, the `Bug` it fixes in a sentence, and `Apply`, which patches the loaded document. `importer/workarounds` has the interface and the helpers they share (`Operation`, `JSONResponse`, `AddParameters`, `Property`).

`Apply` must first check the bug is there, and return an error when it is not: the parameter it adds already declared, the operation gone, the schema already carrying the field. A refreshed document that fixes the bug then fails the import, naming the workaround to delete, instead of the workaround silently doing nothing forever. `workarounds.Verify` is the test for that, one call in the repository's own tests: every workaround must apply to its vendored document, fail when applied a second time, and return rather than panic on a document that has lost its 200 responses.

Workarounds are for the document's shape, what an operation takes and answers. Behaviour no document could express (a filter the server silently drops, an add it loses mid-refresh) belongs in the hand-written layer over the SDK, next to the live test that found it.

## Definitions

`<dir>/<service>-<version>/Service.json` names the package, the document's title and version, the auth and the workarounds applied; `<Group>.json` holds a tag's operations, models and constants. Operations are sorted by name, fields by JSON name, enum values and options in document order, so a spec refresh is a readable diff. The generator reads nothing else, and validates what it reads (every reference resolves, no two operations collide), so the definitions can be reviewed, and in a pinch hand-edited, without the document.

## Diffing

`diff` compares the checked-in definitions with a fresh import of each document, or two definitions directories with `-old` and `-new`, and prints what changed in API terms, for example:

```
emby: 1 added, 0 removed, 2 changed (1 breaking)
  document: Emby Server API 4.1.1.0 -> Emby Server API 4.10.0.0
+ operation GetItemsFilters3 (GET /Items/Filters3)
~ operation GetItems (GET /Items)
    + option query searchterm: String
    ~ expected status codes [200] -> [200 204]
~ model BaseItemDto
    ~ field RunTimeTicks: Integer -> Integer64 [breaking]
```

A change is breaking when it would break a caller of the generated SDK: a removal, a changed type or name, a new request body, a status code no longer expected, a list that changes how it travels or how it pages, a number held by pointer or no longer. A reworded description or a nullable flag is reported and not breaking: the generated code is the same.

## Generating

`generate` writes one package per service, with a file per operation, per model and per tag's enums:

```
client.go                                 Client, New, APIVersion, DocumentVersion
client_test.go                            the canned servers the generated tests share
doc.go                                    package documentation, the workarounds applied
<tag>_method_<operation>.go               the method, <Name>OperationResponse,
                                          <Name>OperationOptions, and for a list
                                          <Name>Complete and <Name>CompleteResult
<tag>_method_<operation>_test.go          the operation's generated test
<tag>_model_<model>.go                    one model
<tag>_constants.go                        the tag's enums, with PossibleValuesFor<Enum>
```

Every generated file starts with the generated-code header; files with it that generation no longer produces are deleted, and anything without it is left alone, so hand-written files can live beside the generated ones.

A method takes `ctx`, the path parameters, the body (`input`: the model by value, or an `io.Reader` and a content type), then the options struct by value, and returns `(result <Name>OperationResponse, err error)`:

```go
res, err := c.GetItems(ctx, emby.GetItemsOperationOptions{Recursive: new(true), Limit: new(50)})
res.HttpResponse // set whenever the server answered, its body readable again
res.Model        // *emby.QueryResultBaseItemDto

all, err := c.GetItemsComplete(ctx, emby.GetItemsOperationOptions{Recursive: new(true)})
all.Items        // every page
```

- Options are sent only when set: a boolean or a number is a pointer, so false, season 0 and image 0 can be asked for; an empty string or list is skipped.
- In models, booleans are `*bool` and lists and maps are `omitzero`, so a body can leave a flag to the server's default, send an explicit false, and clear a list with an empty one.
- Numbers in models are held by value, a null read as 0, except the fields `KeepNull` names, which are pointers.
- `Model` is a pointer for a struct, enum or primitive, and the value for a list, map or raw JSON. An operation that answers a file has no `Model`: its body is left unread in `HttpResponse.Body` for the caller to read and close. An operation that expects a 204 beside its JSON answers a nil `Model` and no error for it.
- A status outside `ExpectedStatusCodes` is an error from the base client, even a 2xx, returned with the response.
- A `Complete` pager asks for the page size the options give (`client.DefaultPageSize` when unset) and stops on an empty or short page, or at the total when the server reports one.

### Generated tests

Every operation gets a test against a canned server, generated from its definition: it calls the method with a sample value for every path parameter (a slash in string ones, to prove escaping) and every option, and checks the server saw the method, the escaped path, each option on the wire, the headers, and the body and its content type. The server answers the first documented status with a sample of the documented response, which must decode (or stream, for a file); then an answer that does not decode, and a status the operation does not document, must both come back as errors with the response. A list's `Complete` must walk its pages. That exercises about 95% of a generated package without a server.

They prove the generated code does what the definitions say, not that the definitions are right about the server: that takes tests against the real thing.

### The base client

Generated code sends every request through a hand-written base client, which each repository keeps beside its generated packages, after go-azure-sdk's `sdk/client`. It stays with the repository because it is where a service's own behaviour belongs: how it takes a credential, what it retries, how it words a failure. What generated code needs of it:

- `Client`, with `BaseURL string` and `HTTPClient *http.Client`, and `New(baseURL string, authorizer Authorizer) (*Client, error)`, which refuses a URL with no scheme and trims a trailing slash
- an authorizer the service's `Auth` names, as a function or a type that takes the token
- `RequestOptions` with `ContentType`, `ExpectedStatusCodes`, `HTTPMethod`, `OptionsObject`, `Path` and `StreamResponse`
- `(*Client).NewRequest`, and on its request `Marshal`, `SetBody` and `Execute`
- a `Response` that embeds `*http.Response`, is returned beside an error whenever the server answered, and has `Unmarshal`
- `Headers` and `QueryParams`, each with `Append`
- `CSV`, `JSONObject`, `DeepObject`, `DefaultPageSize`, and `StatusCode(err)`, which reads the status from the error for an undocumented one and answers 0 for any other

[`internal/client`](internal/client/client.go) is the smallest client that meets it, and what this module's tests build and run generated packages against. It is the contract in code, not a client to ship.

## What was left out of Pandora

Resource ID types and parsers (the servers' ids are opaque strings), per-model predicates for `CompleteMatchingPredicate`, `Default<Name>OperationOptions` constructors (the zero value is the default), long-running operation pollers, API versions, the Terraform and documentation generators, and the data API server: a handful of documents in one repository need none of it.

## Development

```bash
make tools      # build the pinned dev tools into .tools/bin
make check-all  # build + test + lint + actionlint + yamllint + shellcheck + typos + depscheck
make cover      # tests with coverage; the coverage workflow publishes the total as the README badge
```

Nothing is vendored and nothing outside the standard library is imported; tool versions are pinned in `.tools/go.mod` and dependabot keeps them current.
