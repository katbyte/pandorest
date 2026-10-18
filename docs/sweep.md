# The read sweep

The generated tests cannot say whether the document is right about the server. The `sweep` package is the test that can, for a repository's live suite. It calls every GET in a service's definitions on the generated client against a real server, with arguments resolved from the suite's fixtures, and holds each answer to three things. It decodes. It carries something, because an empty list decodes into any model. And no object in it is lost whole, which is how a model whose fields do not match the server shows up.

```go
sweep.Sweep{
	Definitions: defs,   // definitions.Load of the service's directory
	Client:      sdk,    // the generated client
	StatusCode:  client.StatusCode,
	Fixtures:    sweep.Fixtures{Path: map[string]string{"Items/Id": movieID}, Always: map[string]string{"UserId": userID}},
	Cases: map[string]sweep.Case{
		"GetLiveTvTuners": {Skip: "needs a tuner"},
		"GetItemsByIdThemeMedia": {Empty: "the fixtures have no theme songs"},
	},
	MissingIDs: map[string]sweep.Missing{"ItemID": {ID: "00000000000000000000000000000000", Operation: "GetItem", Want: "404"}},
}.Run(t)
```

A path parameter's value is looked up in `Fixtures.Path` by the literal segments before it and its name, the most segments first, then by its name alone. `/api/v3/config/indexer/{id}` finds `config/indexer/id` before `indexer/id` before `id`. Case does not matter.

An event stream never ends, so the sweep reads as far as the first event and hangs up. One that sends no event in `EventWait`, ten seconds unless set, is held to that like an empty list.

Every operation either answers with something or has a `Case` that says why not: skipped, an error status, an answer that does not decode, or an empty one, each with its reason. A case whose operation starts answering fails the sweep, so a stale case is removed the way a stale workaround is. So does a case that names no GET. `Strict` also fails any key the model has no field for.

At the end of a run the sweep logs what it came to:

```text
mini: 13 GET operations: 9 answered, 1 answered nothing by their Empty case, 2 refused as their case says, 1 skipped, 0 failed
enum ContainerState: 4 places in what a GET answers, a value seen at 3 (never: Stack.State); listed values never seen: dead, removing
```

Of the places an answer can hold an enum, a date or a boolean held by value, it says which ones a value came for, and per enum which listed values no answer carried. These are notes, not failures: a boolean held by value reads false whether the server left it out or sent false, and an enum value no answer carried may be one the server never sends.

`sweep.NeverCalled(defs, calls.Seen(), excused)` is the check for a whole live suite rather than the sweep's GETs. With the suite's client made `client.WithCalls(&calls)`, every request it sent is recorded. At the end each operation none reached must be excused by name with a reason, so the suite fails on one neither called nor excused, and on an excuse that names no operation.

For a service of calls the sweep walks every read call instead, through the generated client by its short name (`c.Pool.Query`), wrapping the client's connection so each answer is read as it came as well as decoded. A call's arguments come from the case's `Arguments` by the argument's name, as a value of the argument's own type or a string to read one from, then from the fixtures' `Always` and, for an argument the server insists on, `Options`; the rest are left as their zero. A refusal is held to the case's `Refused`, the error name the server gives (`EACCES` for a role it does not have), the way an HTTP sweep holds a `Status`. The run's summary counts read calls rather than GET operations.

```go
sweep.Sweep{
	Definitions: defs,
	Client:      sdk, // client.New(conn)
	Fixtures:    sweep.Fixtures{Options: map[string]string{"id": "1"}},
	Cases: map[string]sweep.Case{
		"AuditQuery":          {Arguments: map[string]any{"options": truenas.QueryOptions{Limit: 10}}},
		"ApiKeyQuery":         {Refused: "EACCES", Why: "a read-only key may not list keys"},
		"PoolGetInstance":     {Arguments: map[string]any{"id": 2}},
	},
}.Run(t)
```

`MissingIDs` gives, per kind of id, a value of the right shape nothing has, the GET to probe it through, and what the server should answer: a status such as `"404"`, or `"nothing"` for a success with nothing in it. The run fails when the answer is anything else, so a server that starts answering a missing id differently fails CI rather than changing a line in the log, and notes what it was: `a missing ItemID answers HTTP 404 (GET /Items/{id})`. Without `Operation` the probe goes through the first GET whose path takes the kind, which may be an odd route; without `Want` it only notes.
