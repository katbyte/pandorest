package importer

import (
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/config"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/jsonrpc"
)

// methods is a description of a few JSON-RPC methods as TrueNAS writes its own.
const methods = `{
  "pool.query": {"description": "Query pools.", "roles": ["POOL_READ", "READONLY_ADMIN"], "filterable": true, "job": false,
    "accepts": [
      {"title": "filters", "type": "array", "items": [{}], "_name_": "filters", "_required_": false},
      {"title": "options", "type": "object", "properties": {"get": {"title": "get", "type": "boolean", "_name_": "get", "_required_": false}, "count": {"title": "count", "type": "boolean", "_name_": "count", "_required_": false}, "limit": {"title": "limit", "type": "integer", "_name_": "limit", "_required_": false}}, "_name_": "options", "_required_": false}],
    "returns": [{"title": "result", "anyOf": [{"type": "array", "items": [{"title": "PoolEntry", "type": "object", "properties": {"id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true}, "name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}}]}, {"type": "integer"}], "_name_": "result", "_required_": true}]},
  "pool.create": {"description": "Create a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": true,
    "accepts": [{"title": "pool_create", "type": "object", "properties": {"name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}, "_name_": "pool_create", "_required_": true}],
    "returns": [{"title": "PoolEntry", "type": "object", "properties": {"id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true}, "name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "pool.scrub.run": {"description": "Scrub a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": false,
    "accepts": [{"title": "name", "type": "string", "_name_": "name", "_required_": true}, {"title": "threshold", "type": "integer", "_name_": "threshold", "_required_": false}],
    "returns": [{"title": "result", "type": "null", "_name_": "result", "_required_": true}]},
  "core.bulk": {"description": "Run a method many times.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": true,
    "accepts": [{"title": "method", "type": "string", "_name_": "method", "_required_": true}, {"title": "params", "type": "array", "items": [{"type": "array", "items": [{}]}], "_name_": "params", "_required_": true}],
    "returns": [{"title": "result", "type": "array", "items": [{}], "_name_": "result", "_required_": true}]},
  "api_key.delete": {"description": "Delete a key.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false,
    "accepts": [{"title": "id", "type": "integer", "_name_": "id", "_required_": true}],
    "returns": [{"title": "result", "type": "boolean", "_name_": "result", "_required_": true}]},
  "auth.login_ex": {"description": "Authenticate.", "roles": [], "filterable": false, "job": false, "no_auth_required": true,
    "accepts": [{"title": "login_data", "discriminator": {"propertyName": "mechanism"}, "oneOf": [
      {"title": "AuthApiKeyPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "API_KEY_PLAIN", "enum": ["API_KEY_PLAIN"], "_name_": "mechanism", "_required_": true}, "api_key": {"title": "api_key", "type": "string", "_name_": "api_key", "_required_": true}}},
      {"title": "AuthPasswordPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "PASSWORD_PLAIN", "enum": ["PASSWORD_PLAIN"], "_name_": "mechanism", "_required_": true}, "password": {"title": "password", "type": "string", "_name_": "password", "_required_": true}}}],
      "_name_": "login_data", "_required_": true}],
    "returns": [{"title": "result", "type": "boolean", "_name_": "result", "_required_": true}]},
  "directoryservices.status": {"description": "Status.", "roles": [], "filterable": false, "job": false, "accepts": [],
    "returns": [{"title": "result", "type": "object", "properties": {"state": {"title": "state", "type": "string", "enum": ["HEALTHY", "FAULTED"], "_name_": "state", "_required_": true}}, "_name_": "result", "_required_": true}]}
}`

func callService(t *testing.T, cfg config.Service) *definitions.Service {
	t.Helper()

	spec, _, err := jsonrpc.Parse([]byte(methods), "TrueNAS", "25.10.7")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	return svc
}

// A method is an operation named for it, in the group of its namespace, with its arguments in order and typed, the record a
// parameter takes a model named for the parameter, its answer the method's, and a read where the role the config names may
// call it or the config says so.
func TestCallOperations(t *testing.T) {
	t.Parallel()

	svc := callService(t, config.Service{Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, ReadRole: "READONLY_ADMIN", Reads: []string{"directoryservices.status"}, NotReads: []string{"api_key.delete"}})
	ops := map[string]*definitions.Operation{}
	groups := map[string]string{}
	for i := range svc.Groups {
		for j := range svc.Groups[i].Operations {
			o := &svc.Groups[i].Operations[j]
			ops[o.Name] = o
			groups[o.Name] = svc.Groups[i].Name
		}
	}
	query := ops["PoolQuery"]
	if query == nil || query.Method != "CALL" || query.Path != "pool.query" || query.Key() != "CALL pool.query" || groups["PoolQuery"] != "Pool" || query.Description != "Query pools." {
		t.Fatalf("PoolQuery = %+v in %s", query, groups["PoolQuery"])
	}
	if query.Call == nil || query.Call.Method != "pool.query" || !query.Call.Filterable || !query.Call.Read || query.Call.Job {
		t.Errorf("PoolQuery's call = %+v", query.Call)
	}
	if len(query.Arguments) != 2 || query.Arguments[0].Argument != "filters" || query.Arguments[0].Type.Type != definitions.List || query.Arguments[0].Required || query.Arguments[1].Argument != "options" || query.Arguments[1].Type != reference("PoolQueryOptions") {
		t.Errorf("PoolQuery's arguments = %+v", query.Arguments)
	}
	if query.Response == nil || query.Response.Type.Type != definitions.List || *query.Response.Type.NestedItem != reference("PoolEntry") || len(query.ExpectedStatusCodes) != 0 || len(query.Options) != 0 {
		t.Errorf("PoolQuery's answer = %+v", query.Response)
	}

	// a query's one-record and count forms are operations of their own, over the same method, with the options always sent
	one, count := ops["PoolQueryOne"], ops["PoolQueryCount"]
	if one == nil || count == nil || one.Call.Form != definitions.QueryGet || count.Call.Form != definitions.QueryCount || one.Key() != "CALL pool.query#get" || one.Call.Method != "pool.query" || groups["PoolQueryOne"] != "Pool" {
		t.Fatalf("the forms = %+v, %+v", one, count)
	}
	if one.Response.Type != reference("PoolEntry") || count.Response.Type.Type != definitions.Integer || !one.Arguments[1].Required || query.Arguments[1].Required || !one.Call.Read {
		t.Errorf("the forms' answers and arguments = %+v, %+v", one, count)
	}
	if svc.Models()["PoolQueryOptions"] == nil || slices.ContainsFunc(svc.Models()["PoolQueryOptions"].Fields, func(f definitions.Field) bool { return f.JSONName == "count" }) {
		t.Errorf("the options keep the count flag: %+v", svc.Models()["PoolQueryOptions"])
	}

	// a choice of records is raw JSON, or with TypedUnions a union model named for its place, its members told apart by the discriminator
	if login := ops["AuthLoginEx"]; login.Arguments[0].Type.Type != definitions.RawObject {
		t.Errorf("the login data without TypedUnions = %+v", login.Arguments[0].Type)
	}
	typed := callService(t, config.Service{Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, TypedUnions: true})
	union := typed.Models()["AuthLoginExLoginData"]
	if union == nil || union.Discriminator != "mechanism" || len(union.Members) != 2 || union.Members[0] != (definitions.Member{Model: "AuthApiKeyPlain", Kind: "API_KEY_PLAIN"}) || union.Members[1] != (definitions.Member{Model: "AuthPasswordPlain", Kind: "PASSWORD_PLAIN"}) || len(union.Union) != 2 {
		t.Errorf("the login data with TypedUnions = %+v", union)
	}
	for _, o := range typed.Operations() {
		if o.Name == "AuthLoginEx" && o.Arguments[0].Type != reference("AuthLoginExLoginData") {
			t.Errorf("the login argument = %+v", o.Arguments[0].Type)
		}
	}
	if create := ops["PoolCreate"]; create.Call.Read || !create.Call.Job || create.Arguments[0].Argument != "poolCreate" || create.Arguments[0].Type != reference("PoolCreate") || !create.Arguments[0].Required || create.Response.Type != reference("PoolEntry") {
		t.Errorf("PoolCreate = %+v", create)
	}
	if scrub := ops["PoolScrubRun"]; groups["PoolScrubRun"] != "PoolScrub" || scrub.Response != nil || len(scrub.Arguments) != 2 || scrub.Arguments[1].Argument != "threshold" || scrub.Arguments[1].Required {
		t.Errorf("PoolScrubRun = %+v", scrub)
	}
	// an argument named as the method's own locals are is kept clear of them
	if bulk := ops["CoreBulk"]; bulk.Arguments[1].Argument != "paramsParam" || bulk.Arguments[1].Name != "params" {
		t.Errorf("CoreBulk's arguments = %+v", bulk.Arguments)
	}
	if del := ops["ApiKeyDelete"]; del.Call.Read {
		t.Error("ApiKeyDelete is a read, though NotReads names it")
	}
	if status := ops["DirectoryservicesStatus"]; !status.Call.Read || status.Response.Type != reference("DirectoryservicesStatusAnswer") {
		t.Errorf("DirectoryservicesStatus = %+v, %+v", status.Call, status.Response)
	}
	if svc.Constants()["DirectoryservicesStatusAnswerState"] == nil {
		t.Error("the enum in an inline answer has no type")
	}
	if err := definitions.Validate(svc); err != nil {
		t.Error(err)
	}
}

// A name in Reads or NotReads that is no method of the document fails the import, so the lists cannot go stale.
func TestCallReadsRefused(t *testing.T) {
	t.Parallel()

	spec, _, err := jsonrpc.Parse([]byte(methods), "TrueNAS", "25.10.7")
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]config.Service{
		"Reads":    {Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, Reads: []string{"pool.ghost"}},
		"NotReads": {Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, NotReads: []string{"pool.ghost"}},
	} {
		_, err := FromSpec(cfg, spec, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "the config's "+name+" names pool.ghost, which is no method of the document") {
			t.Errorf("%s: FromSpec = %v", name, err)
		}
	}
}
