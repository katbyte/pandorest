# Importing, workarounds, definitions and diffs

## What import does

`import` loads each document, applies its workarounds, and normalises it.

- **Names.** With `OperationIDNaming` a method is named after its operationId (`GetItems`; TMDB's `movie-details` is `MovieDetails`). With `PathNaming`, for operationIds that are machine-made or missing, it is named after the method and path: `GET /Items/{Id}/Similar` is `GetItemsByIdSimilar`. Schema names lose their dots and underscores. Fields keep the API's spelling (`Id`, `ImdbId`). A field with a leading underscore beside its plain twin takes an `Underscore` prefix.
- **Types.** `Integer`, `Integer64`, `Float`, `Double`, `String`, `Boolean`, `List`, `Dictionary`, `Reference` to a model or enum, `RawObject` for JSON of no declared shape, `Any`, and `RawFile` for bodies that are not JSON. An inline object becomes a model named after its owner and field. An operation's inline request and response are named after the operation.
- **Operations.** Path parameters in template order. Query and header parameters as options, with lists comma-separated or one key per value as the document says. A header named `Accept`, `Content-Type` or `Authorization` is ignored, as OpenAPI says, unless a workaround added it. Then the request body, the success response, the expected status codes, and whether the operation pages.
- **Calls.** For a JSON-RPC document, each method is an operation whose arguments are in the order the server takes them, with no path, options or status codes. Its namespace is its tag.
- **Grouping.** One definitions file per tag, holding the tag's operations and the models and enums only its operations use. Anything more than one tag uses is in `Common.json`.

The importer is strict. An undeclared path parameter, a GET that does not say what it answers, a duplicate operationId, two operations that would make the same method name, an operation without a tag or a success response, or two schemas that would declare the same Go type each fail the import, with every problem listed.

## Workarounds

A document bug is fixed with a workaround, kept in the repository that vendors the document. It is a type with a `Name` (`emby-search-term`), the `Service` it is for, the `Bug` it fixes in a sentence, and `Apply`, which patches the loaded document. `importer/workarounds` has the interface and the helpers they share (`Operation`, `JSONResponse`, `AddParameters`, `Property`).

`Apply` must first check the bug is there, and return an error when it is not. A refreshed document that fixes the bug then fails the import naming the workaround to delete, instead of the workaround silently doing nothing forever. `workarounds.Verify` is the test for that, one call in the repository's own tests: every workaround must apply to its vendored document, fail when applied a second time, and return rather than panic on a document that has lost its 200 responses.

### Ready-made workarounds

The same few bugs are in most documents, so the workarounds for them are here. A repository says which operations and why; the patch and the check are written once. Each is a `Workaround` like one written by hand and passes `Verify` the same way.

```go
var All = []workarounds.Workaround{
	workarounds.UndeclaredAnswers(workarounds.About{Name: "sonarr-undeclared-responses", Service: "sonarr", Bug: "fourteen GETs declare a 200 with no content, so nothing says whether they answer JSON or a file"}, map[string]workarounds.Answer{
		"GET /api/v3/filesystem":          workarounds.JSON(),
		"GET /api/v3/customformat/schema": workarounds.ListOf("CustomFormatSpecificationSchema"),
		"GET /api/v3/log/file/{filename}": workarounds.File("text/plain"),
	}),
	workarounds.WrongAnswers(workarounds.About{Name: "emby-parent-path-text", Service: "emby", Bug: "GET /Environment/ParentPath declares a JSON string; the server answers the bare path as text"}, map[string]workarounds.Correction{
		"GET /Environment/ParentPath": {Declared: workarounds.JSONString(), Answers: workarounds.Text("text/plain")},
	}),
}
```

| Workaround | The bug it is for | It fails when |
|---|---|---|
| `UndeclaredAnswers` | operations declare a success with no content, so nothing says whether they answer JSON, and in what shape, or a file | one declares its answer now |
| `WrongAnswers` | operations declare one answer and the server gives another: a JSON string sent as bare text, one record where a list is sent | one no longer declares what its correction says |
| `WrongStatus` | operations are documented as one status and answer another with the same body | one no longer documents the declared status, or documents the other |
| `UndeclaredParameters` | operations read a parameter the document leaves out | one declares it now |
| `UndeclaredProperties` | the server sends, and reads back, fields a schema leaves out | the schema declares one now |
| `UndeclaredDates`, `UndeclaredDays` | the server writes a date-time, or a day, in a property or parameter the document declares a plain string. Every string property of the name is declared one. The `Except` forms leave the name alone where it is a string of something else | no string property has the name, every one is declared a date now, or an exception excludes nothing |
| `UndeclaredUnixDates` | the server writes a date as seconds or milliseconds since 1970 in a property the document declares an integer. The package holds it as `Unix` or `UnixMilli` | no integer property has the name, or the unit is not `seconds` or `milliseconds` |
| `UndeclaredEnums` | the server draws a plain string's values from a fixed set the document does not list. Each becomes a typed enum, named for the property unless `Name` says otherwise, everywhere unless `In` narrows it. A path inside a place picks one of two fields of the name in one answer (`"GET /api/stacks .status"` for the top-level one, `"GET /api/stacks containerDetails.status"` for the nested one). The sweep fails an answer carrying a value not listed | no plain string has the name, the enum has no values, or its type name is another schema's |
| `EnumValues` | the server answers a value a declared enum does not list. Each named schema gains the values | a schema is not there, not an enum of strings, or lists a value already |
| `JoinedLists` | the server reads a string parameter as several values joined by commas (Emby's `IncludeItemTypes`). A list is added beside the string as `<Field>List`, sent under the same name joined. `In` narrows it to some operations | no plain string parameter has the name, the element is not an enum of the document or `string`, or an `In` matches nothing |
| `NumberedPathIDs` | the document types a path parameter as text on one operation and as a number on another of the same path. The text one becomes the number | it reaches nothing |
| `NotAPI` | the document lists paths that are the web interface, not the API | one is not in the document |
| `DeclaredShapes` | the document leaves a field open, a record of no declared fields or a value of no type, that the server writes in a stable shape a program reads (a scrub's state). Each field named `Schema.property` takes the schema given: a record declared in place becomes a model named for the place, or a reference points at one `DeclaredSchemas` added | a schema or property is not there, or the document gives the field a shape now |
| `NamedTypes` | the server writes a field in a form of its own the document cannot describe: a secret it hides from some callers behind a placeholder whatever the field's type, a list it writes two ways. Each field named `Schema.property` is held as the Go type given, written as a package uses it and imported from the path given; the type reads and writes its own JSON. `client.Secret[T]` is the one for a hidden value, and `$item` in a type stands for the field's own type or a list's element as the package names it (`truenasrpc.Set[$item]`), so the workaround survives a renamed enum | a schema or property is not there |
| `DeclaredSchemas` | the document leaves a record out altogether, which a workaround then refers to by name. Each schema is added | a name the document has |
| `AlsoAnswers` | an operation answers a body at a status the document gives none. Each status named gets the answer. With `ExpectSameShape` a failure status answering the success's shape is decoded as an answer | a status declares a body already |
| `Commands` | the server queues work by name through one operation whose body is an open record (the *arr's `POST /api/v3/command`). Each command named becomes a `<Name>Command` model, a method that fixes the name, an `AndWait` form that polls through the set's `Get` until its `Status` is one of `Finished`, and `WaitForCommand(ctx, id, wait)` for one queued the plain way. `CommandPoll` is how often it asks | an operation is not there, the queueing one takes no JSON body, no finished values, no commands, a command named like a Go keyword or with a field called `name`, a schema the document has, or a set declared twice |
| `OneRecord` | the document declares the same record inline wherever it appears, so the package has a model per place. Each name becomes one schema every named place refers to | a name the document has a schema of, a place matching no inline object, fewer than two places, or a place whose record differs from the first's |

`ListDeclaredAsOne("Schema")` is the commonest correction: one record declared where a list is sent. An answer is one of `JSON()` (held raw), `JSONString()`, `Model("Schema")`, `ListOf("Schema")`, `Shape(schema)`, `File("media/type")` and `Text("media/type")`. A file is left for its caller to read and close. Text is short text that is not JSON, read whole into a `*string`, whatever the server labels it. Only a workaround makes an answer text.

A workaround can also name an operation by setting its `Name`: the method it gets in place of the one its path or operationId would make. That is how two paths that make the same name are told apart (`GET /audit` beside `GET /api/audit`), since the import fails on the clash rather than number the second.

A workaround can give an operation an `Accept` option, by adding it as a header parameter (`AddParameters` with `Param{Name: "Accept", In: openapi.InHeader}`). It is for a server that chooses its answer by the header: asked for JSON alone, Dockhand's slow actions do the work and then answer, and asked for anything else they answer at once with a job to follow. Left empty the call asks as it would have; set, its value is the whole header.

Workarounds are for the document's shape. Behaviour no document could express (a filter the server silently drops) belongs in the hand-written layer over the generated code, next to the live test that found it.

## Definitions

`<dir>/<service>-<version>/Service.json` names the package, the document's title and version, the auth and the workarounds applied. `<Group>.json` holds a tag's operations, models and constants; a group named `Service` goes in `Service.group.json`. Operations are sorted by name, fields by JSON name, enum values and options in document order, so a refresh is a readable diff. The generator reads nothing else, and validates what it reads, so the definitions can be reviewed and in a pinch hand-edited.

## Diffing

`diff` compares the checked-in definitions with a fresh import of each document, or two definitions directories with `-old` and `-new`, and prints what changed in API terms:

```text
emby: 1 added, 0 removed, 2 changed (1 breaking)
  document: Emby Server API 4.1.1.0 -> Emby Server API 4.10.0.0
+ operation GetItemsFilters3 (GET /Items/Filters3)
~ operation GetItems (GET /Items)
    + option query searchterm: String
    ~ expected status codes [200] -> [200 204]
~ model BaseItemDto
    ~ field RunTimeTicks: Integer -> Integer64 [breaking]
```

A change is breaking when it would break a caller of the generated code: a removal, a changed type or name, a new request body, a status code no longer expected, a list that changes how it travels or pages, a number held by pointer or no longer. A reworded description or a nullable flag is reported and not breaking.
