package sweep_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/sweep"
)

// A fake package of calls, laid out as a generated grouped one: a client with a field per group, each group's client over the
// shared connection, and each method dropping the group's name.
type pool struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type poolOptions struct {
	Limit int `json:"limit,omitempty"`
}

type poolList struct{ Answer []pool }

type poolOne struct{ Answer *pool }

type poolClient struct{ Caller client.Caller }

func (c poolClient) Query(ctx context.Context, filters []any, options poolOptions) (result poolList, err error) {
	err = c.Caller.Call(ctx, "pool.query", client.CallParams(0, filters, options), &result.Answer)

	return result, err
}

func (c poolClient) GetInstance(ctx context.Context, id int) (result poolOne, err error) {
	var answer pool
	err = c.Caller.Call(ctx, "pool.get_instance", client.CallParams(1, id), &answer)
	if err == nil {
		result.Answer = &answer
	}

	return result, err
}

func (c poolClient) Secrets(ctx context.Context) (result poolList, err error) {
	err = c.Caller.Call(ctx, "pool.secrets", nil, &result.Answer)

	return result, err
}

func (c poolClient) Odd(ctx context.Context) (result poolList, err error) {
	err = c.Caller.Call(ctx, "pool.odd", nil, &result.Answer)

	return result, err
}

func (c poolClient) Create(ctx context.Context, data pool) (result poolOne, err error) {
	err = c.Caller.Call(ctx, "pool.create", client.CallParams(1, data), &result.Answer)

	return result, err
}

type callClient struct {
	Caller client.Caller
	Pool   poolClient
}

// nas answers as a server would, recording each call.
type nas struct{ calls []string }

func (n *nas) Call(_ context.Context, method string, params []any, into any) error {
	p, _ := json.Marshal(params)
	n.calls = append(n.calls, method+" "+string(p))
	var body string
	switch {
	case method == "pool.query":
		body = `[{"name": "tank", "size": 7, "extra": true}]`
	case method == "pool.get_instance" && len(params) == 1 && params[0] == 1:
		body = `{"name": "tank", "size": 7}`
	case method == "pool.get_instance":
		return &client.CallError{Method: method, Code: -32001, Errno: 2, ErrName: "ENOENT", Reason: "not there"}
	case method == "pool.secrets":
		return &client.CallError{Method: method, Code: -32001, Errno: 13, ErrName: "EACCES", Reason: "Not authorized"}
	case method == "pool.odd":
		body = `{"name": "not a list"}`
	default:
		return errors.New("no such method")
	}

	return json.Unmarshal([]byte(body), into)
}

func callOp(name, method string, read bool, answer definitions.TypeRef, args ...definitions.Argument) definitions.Operation {
	return definitions.Operation{Name: name, Method: "CALL", Path: method, Call: &definitions.Call{Method: method, Read: read}, Arguments: args, Response: &definitions.Body{ContentType: "application/json", Type: answer}}
}

func callService() *definitions.Service {
	list := definitions.TypeRef{Type: definitions.List, NestedItem: &definitions.TypeRef{Type: definitions.Reference, ReferenceName: "Pool"}}
	one := definitions.TypeRef{Type: definitions.Reference, ReferenceName: "Pool"}

	return &definitions.Service{Name: "nas", Groups: []definitions.Group{{Name: "Pool", Operations: []definitions.Operation{
		callOp("PoolQuery", "pool.query", true, list, definitions.Argument{Name: "filters", Argument: "filters", Type: definitions.TypeRef{Type: definitions.List, NestedItem: &definitions.TypeRef{Type: definitions.Any}}}, definitions.Argument{Name: "options", Argument: "options", Type: definitions.TypeRef{Type: definitions.Reference, ReferenceName: "PoolOptions"}}),
		callOp("PoolGetInstance", "pool.get_instance", true, one, definitions.Argument{Name: "id", Argument: "id", Type: definitions.TypeRef{Type: definitions.Integer}, Required: true}),
		callOp("PoolSecrets", "pool.secrets", true, list),
		callOp("PoolOdd", "pool.odd", true, list),
		callOp("PoolCreate", "pool.create", false, one, definitions.Argument{Name: "data", Argument: "data", Type: definitions.TypeRef{Type: definitions.Reference, ReferenceName: "Pool"}, Required: true}),
	}}}}
}

// The sweep calls every read of a service of calls through the generated client by its short name, with the arguments the
// fixtures and cases give, reads each answer as it came, and holds a refusal to the error name its case expects.
func TestSweepCalls(t *testing.T) {
	t.Parallel()

	n := &nas{}
	sdk := &callClient{Caller: n, Pool: poolClient{Caller: n}}
	s := sweep.Sweep{
		Definitions: callService(),
		Client:      sdk,
		Fixtures:    sweep.Fixtures{Options: map[string]string{"id": "1"}},
		Cases: map[string]sweep.Case{
			"PoolSecrets": {Refused: "EACCES", Why: "the read-only user may not"},
			"PoolOdd":     {Decode: true, Why: "answers one record where its description says a list"},
		},
	}
	ops := map[string]*definitions.Operation{}
	for _, op := range s.Definitions.Operations() {
		ops[op.Name] = op
	}

	if out := s.Operation(t.Context(), ops["PoolQuery"]); len(out.Failures) != 0 || out.Skipped != "" {
		t.Errorf("PoolQuery = %+v", out)
	}
	if out := s.Operation(t.Context(), ops["PoolGetInstance"]); len(out.Failures) != 0 {
		t.Errorf("PoolGetInstance = %+v", out)
	}
	if out := s.Operation(t.Context(), ops["PoolSecrets"]); len(out.Failures) != 0 || len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "refused with EACCES, as expected") {
		t.Errorf("PoolSecrets = %+v", out)
	}
	if out := s.Operation(t.Context(), ops["PoolOdd"]); len(out.Failures) != 0 || len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "does not decode, as expected") {
		t.Errorf("PoolOdd = %+v", out)
	}
	if want := []string{"pool.query null", "pool.get_instance [1]", "pool.secrets null", "pool.odd null"}; !reflect.DeepEqual(n.calls, want) {
		t.Errorf("calls = %q, want %q", n.calls, want)
	}
	if sdk.Pool.Caller != client.Caller(n) {
		t.Error("the sweep swapped the client's own connection")
	}

	// a case's arguments win, and a refusal no case expects is a failure naming it
	n.calls = nil
	s.Cases["PoolGetInstance"] = sweep.Case{Arguments: map[string]any{"id": 2}}
	if out := s.Operation(t.Context(), ops["PoolGetInstance"]); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "ENOENT") || n.calls[0] != "pool.get_instance [2]" {
		t.Errorf("PoolGetInstance of 2 = %+v, calls %q", out, n.calls)
	}
	s.Cases["PoolQuery"] = sweep.Case{Arguments: map[string]any{"filters": []any{[]any{"name", "=", "tank"}}, "options": poolOptions{Limit: 1}}}
	if out := s.Operation(t.Context(), ops["PoolQuery"]); len(out.Failures) != 0 || n.calls[1] != `pool.query [[["name","=","tank"]],{"limit":1}]` {
		t.Errorf("PoolQuery with arguments = %+v, calls %q", out, n.calls)
	}

	// a required argument nothing gives is an error, a case that now answers is stale, and so is one for a write
	s.Fixtures = sweep.Fixtures{}
	delete(s.Cases, "PoolGetInstance")
	if out := s.Operation(t.Context(), ops["PoolGetInstance"]); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "no value for the argument id") {
		t.Errorf("PoolGetInstance without an id = %+v", out)
	}
	s.Cases["PoolQuery"] = sweep.Case{Refused: "EACCES", Why: "it is not"}
	if out := s.Operation(t.Context(), ops["PoolQuery"]); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "now answers; drop its case") {
		t.Errorf("a stale refusal case = %+v", out)
	}
	s.Cases["PoolCreate"] = sweep.Case{Skip: "a write"}
	if stale := s.StaleCases(); !reflect.DeepEqual(stale, []string{"PoolCreate"}) {
		t.Errorf("StaleCases = %v", stale)
	}
}
