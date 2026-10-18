# The generated code

`generate` writes a service's package in one of two layouts.

## Grouped

Grouped (`Grouped: true`) is laid out the way go-azure-sdk lays out a service, as packages. A caller names every type as `emby.X`, reaches every operation through one client, and never imports a tag's package, whose name is then free to be a word the caller uses for a variable:

```text
sdk/emby/                      package emby:  every model (model_<model>.go), every enum (constants_<tag>.go),
                                              every operation's options and response types (operation_<operation>.go),
                                              New for the base client (client.go), the base client's options and
                                              error helpers under this name (helpers.go), and the service's own service.go
sdk/emby/library/              package library: Client and NewClient (client.go), a method per operation
                                              (method_<operation>.go) and its test, the canned server (client_test.go)
sdk/emby/client/               package client: New, and Client with a field per tag (client.go)
sdk/emby/groups.json           each operation's tag and each tag's package, for a caller moving from the flat layout
```

```go
import (
	"github.com/katbyte/embyfin-mcp/sdk/emby"
	"github.com/katbyte/embyfin-mcp/sdk/emby/client"
)

c, err := client.New(url, key, emby.WithLog(log))
res, err := c.Library.GetItemsByIdAncestors(ctx, id, emby.GetItemsByIdAncestorsOptions{UserId: user})
var parent emby.BaseItemDto = res.Answer[0]
```

A package's name matches its directory. Under a version directory (`Outputs`) the types package is a directory of its own: `sdk/emby/4.10.0.40/emby`, `sdk/emby/4.10.0.40/library`, `sdk/emby/4.10.0.40/client`. A repository whose files import pandorest's own `client` beside it may name the package with `ClientPackage` (`embyclient`). A tag whose lowercased name is a Go keyword, a standard library package, `client` or the package's own name fails the generation and is named in `Packages`. Method names stay unique across the whole, so the sweep and a repository's own checks go by the bare name. The packages import one another under the repository's module path, read from `go.mod` or set as `Module`.

Moving from the flat layout is mostly a script over `groups.json`: `Methods` names each method's tag and `Groups` each tag's package and field, so `c.GetItems(` becomes `c.Items.GetItems(`. By hand: the service's own file moves to the types package, a hand-written test of the whole client moves to the client package, and code that built the client by hand goes once `ClientService`, `Auth` and `Credential` describe it.

## Flat

Flat, the default, writes one package per service, with a file per operation, per model and per tag's enums:

```text
client.go                                 Client, New, APIVersion, DocumentVersion
client_test.go                            the canned servers the generated tests share
doc.go                                    package documentation, the workarounds applied
<tag>_method_<operation>.go               the method, <Name>Response, <Name>Options, and for a list <Name>Complete
<tag>_method_<operation>_test.go          the operation's generated test
<tag>_model_<model>.go                    one model
<tag>_constants.go                        the tag's enums, with PossibleValuesFor<Enum>
```

Every generated file starts with the generated-code header. Files with it that generation no longer produces are deleted. Anything without it is left alone, so hand-written files live beside the generated ones, and one at a name generation writes to is refused rather than overwritten. The exception is `doc.go`, which starts with the package comment and says there that pandorest generated it, so a linter does not tell a package with a hand-written file that it has no package comment. `generator.Generated` tells a generated file from one written by hand.

What is generated passes the linters a repository runs over its own code, so the generated-code exclusion (`linters.exclusions.generated: disable` in golangci-lint's config) can be turned off and the package linted like the rest. pandorest holds itself to that: `make lint-generated` renders the packages its tests cover and lints them with the exclusion off, and CI runs it. Three things stay for a repository's config to exclude for its generated packages, as pandorest's own does for `generator/testdata`: `goconst`, since the operations and the tests repeat the document's words by design; gocritic's `hugeParam` and `rangeValCopy`, since a method takes a request body by value whatever its size, so a caller can pass a literal; and gosec's `G101` where a document names an enum value with a word like key or token.

## A method

A method takes `ctx`, the path parameters, the body (`input`: the model by value, or an `io.Reader` and a content type), then the options struct by value, and returns `(result <Name>Response, err error)`. When nothing in the options is required the struct may be left out: `c.GetSessions(ctx)`. Two structs are refused before any request.

With `InlineSingleOptions` an operation with one option takes it as an argument: `c.GetMovieLookup(ctx, term)` or `c.GetMovieLookup(ctx)`. A required one is plain, an optional one variadic, a list as its elements. An operation with two or more keeps the struct, as does a list that pages.

```go
res, err := c.GetItems(ctx, emby.GetItemsOptions{Recursive: new(true), Limit: new(50)})
res.HttpResponse // set whenever the server answered, its body readable again
res.Answer       // *emby.QueryResultBaseItemDto, also res.Items through the embed

all, err := c.GetItemsComplete(ctx)
all.Items        // every page
```

- Options are sent only when set. A boolean or a number is a pointer, so false and 0 can be asked for. An empty string or list is skipped.
- In models, booleans are `*bool` and lists and maps are `omitzero`, so a body can leave a flag to the server's default, send an explicit false, and clear a list with an empty one.
- Numbers in models are held by value, a null read as 0, except the fields `KeepNull` names.
- `Answer` is a pointer for a struct, enum or primitive, and the value for a list, map or raw JSON. `Answered()` says whether a body came and was decoded. A struct answer is also embedded, so `res.Answer.Name` may be read as `res.Name`, a zero record when nothing was answered. `res.Record()` is the record as a value, or an error when none was answered.
- An operation that answers a file has no `Answer`. Its body is left unread in `HttpResponse.Body`. `WriteTo(w)` copies it out and closes it. `Bytes(max)` reads it whole, failing rather than cutting short a file over `max`. `Reader()` hands the body over with its content type. `ContentLength()` and `Filename()` say what the server said of it. An event stream has `Reader` and `ContentLength` only.
- A status outside the expected ones is a `*client.StatusError`, returned with the response. A redirect an operation documents is not an error; see [the base client](base-client.md).
- A `Complete` pager asks for the page size the options give and stops on an empty or short page, or at the total. `Each` walks the same pages as a `range` over `iter.Seq2[Item, error]`, so a loop that stops asks for no page more.
- A record with a `Name` and an `Id` has `GetName()` and `GetId()`, so code that handles records of several types takes them through one interface.
- An answer the document gives no shape to is `json.RawMessage`. `client.Decode[T](raw)` reads it into a type of the caller's own, and `client.DecodeStrict[T]` fails naming a key the type has no field for.

## A call

For a JSON-RPC service each method of the server is a method here, taking a context and then the method's arguments in the order the server takes them. In the grouped layout it sits in its namespace's package under its own name, so `pool.query` is `c.Pool.Query` and `pool.create` is `c.Pool.Create`, while its operation, and so its response type, keeps the whole name: `truenas.PoolQueryResponse`. In the flat layout, where every method is on one client, it is `PoolQuery`. `check -uncalled` counts a call by its group and name together, `c.Pool.Query`, since `Query` alone is every namespace's. A repository with two services, where one's `GetItems` would count for the other's, counts through the field each client sits on with `generator.UncalledOn(defs, root, "emby")`: `s.emby.GetItems` and `s.emby.Items.GetItems` name the method, `s.jf.GetItems` does not.

A method whose answer is a file has a second form, `<Name>To`, that takes a writer before the arguments and writes the file to it through the connection, answering how many bytes: `c.Config.SaveTo(ctx, w, options)`. One that takes files beside its arguments takes them as readers at the end, `c.Filesystem.Put(ctx, path, options, file)`, and called with none is a plain call, which is how the four that take a file only sometimes are called in the usual way. Both need a connection that can carry a file (see [the base client](base-client.md)).

A job's method waits for the job, because the connection asked the server to answer when it is done. For a connection of the repository's own that starts a job and follows it instead, each job method has `<Name>Call` beside it, a function answering the method's name and its arguments as the method would send them: `method, params := pool.ScrubCall("tank", truenas.PoolScrubActionStart)`.

The types package has `Operations`, a table of every operation for a program to go by rather than the comments: the operation's name, its HTTP method and path or the server's method, the group whose client has it, whether it changes nothing, whether it runs as a job, a query's form, and whether its answer is a file or it takes one. A read-only guard, a count of what a test suite called, or a sweep of the repository's own reads it. Every package has it, REST or JSON-RPC.

A query's filters are a list of conditions, each a list of a field, a comparison and a value, which `client.Where`, `client.Filters` and `client.AnyOf` write out so the nesting is never typed by hand: `client.Filters(client.Where("name", "=", "tank"), client.AnyOf(client.Where("size", ">", 10), client.Where("status", "in", []string{"ONLINE"})))`.

An enum met in several records of one namespace under the same property is one type, named by the words the places share (`ReplicationDirection` for the direction of a replication's create, update, query row and answers), so a value read from one record passes to another. The rows of a query are named `<Method>ResultItem`, as the importer names a list's record, and their enums fold with the rest.

A query answers a list, one record or a count by a flag in its options, and each is a method: `Query` answers the list, `QueryOne` the first record the filters match, which the server refuses when they match none, and `QueryCount` how many match. The options lose the two flags, since the method sets the one its form needs, and the options are sent whatever the caller put in them.

```go
pools, err := c.Pool.Query(ctx, nil, truenas.PoolQueryOptions{})            // pools.Answer []truenas.PoolQueryResultItem
tank, err := c.Pool.QueryOne(ctx, []any{[]any{"name", "=", "tank"}}, truenas.PoolQueryOptions{}) // tank.Answer *truenas.PoolQueryResultItem
n, err := c.Pool.QueryCount(ctx, nil, truenas.PoolQueryOptions{})           // n.Answer *int
``` An argument the server does not insist on may be left as its zero, and those left so at the end are not sent; a list or map that is sent unset goes as an empty one, since a server that asks for a list reads null as none. Every call goes through the `client.Caller` the generated `New` is given, which the repository writes over its connection. A call the server refuses is an error carrying a `client.CallError` with the server's code, error name and reason; `IsNotFound` reads `ENOENT`.

## A value that is one of several records

A value the document declares as one of several records is raw JSON for the caller to decode, unless the service sets `TypedUnions`. Then it is a type of its own, holding the record as it came, with a method per member that hands the record over as that member, and `Kind` where the document says which field the records name themselves by: a record of another kind is an error naming both, and without such a field a member of another shape fails on the key the member has no field for. One is built from a member with `client.UnionOf`, which writes the member as given, so the caller sets the field the record names itself by (`Type: "S3"`). A member of a union that is sent is a sent record: its flags are pointers, left out unless set.

```go
provider := entry.Credentials.Provider          // truenas.CloudSyncCredentialsCreateProvider
switch provider.Kind() {
case "S3":
	s3, err := provider.AsS3CredentialsModel()   // truenas.S3CredentialsModel
}
u, err := client.UnionOf(truenas.B2CredentialsModel{Type: "B2", Key: key})
create.Provider = truenas.CloudSyncCredentialsCreateProvider{Union: u}
```

## Generated tests

Every operation gets a test against a canned server. It calls the method with a sample value for every path parameter and option, and checks the server saw the method, the escaped path, each option on the wire, the headers, and the body. The server answers the first documented status with a sample of the documented response, which must decode. Then an answer that does not decode, and a status the operation does not document, must both come back as errors. A list's `Complete` must walk its pages. For a call, a recording connection checks the method name and the arguments sent, and a refusal is passed on.

That exercises most of a generated package without a server. It proves the generated code does what the definitions say, not that the definitions are right about the server. That takes tests against the real thing: see [the read sweep](sweep.md).
