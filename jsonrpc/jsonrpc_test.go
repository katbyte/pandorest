package jsonrpc_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/jsonrpc"
	"github.com/katbyte/pandorest/openapi"
)

// description is a few methods as TrueNAS describes its own: a query, a job that takes a record, a method answering nothing,
// one answering an inline record, a login taking one of several records, two methods whose answers share a title but not a
// shape, and one whose answer is titled as the package names a method's answer.
const description = `{
  "pool.query": {"description": null, "roles": ["POOL_READ", "READONLY_ADMIN"], "filterable": true, "job": false, "item_method": false, "no_auth_required": false,
    "accepts": [
      {"title": "filters", "type": "array", "items": [{}], "default": [], "_name_": "filters", "_required_": false},
      {"title": "options", "type": "object", "additionalProperties": false, "properties": {
        "get": {"title": "get", "type": "boolean", "default": false, "_name_": "get", "_required_": false},
        "count": {"title": "count", "type": "boolean", "default": false, "_name_": "count", "_required_": false},
        "order_by": {"title": "order_by", "type": "array", "items": [{"type": "string"}], "_name_": "order_by", "_required_": false}},
        "_name_": "options", "_required_": false}],
    "returns": [{"title": "result", "anyOf": [
      {"type": "array", "items": [{"title": "PoolEntry", "type": "object", "properties": {
        "id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true},
        "name": {"title": "name", "type": "string", "_name_": "name", "_required_": true},
        "status": {"title": "status", "type": "string", "enum": ["ONLINE", "DEGRADED", ""], "_name_": "status", "_required_": true},
        "scan": {"anyOf": [{"title": "PoolScan", "type": "object", "properties": {"function": {"title": "function", "type": "string", "enum": ["SCRUB", "RESILVER"], "_name_": "function", "_required_": true}}}, {"type": "null"}], "title": "scan", "_name_": "scan", "_required_": true},
        "size": {"anyOf": [{"type": "integer"}, {"type": "null"}], "title": "size", "_name_": "size", "_required_": true}}}]},
      {"title": "PoolEntry", "type": "object", "properties": {"id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true}}},
      {"type": "integer"}], "_name_": "result", "_required_": true}]},
  "pool.update": {"description": "Update a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": true, "item_method": false, "no_auth_required": false,
    "accepts": [{"title": "pool_update", "type": "object", "properties": {"status": {"title": "status", "type": "string", "enum": ["ONLINE", "DEGRADED", ""], "_name_": "status", "_required_": false}}, "_name_": "pool_update", "_required_": true}],
    "returns": [{"title": "result", "type": "boolean", "_name_": "result", "_required_": true}]},
  "pool.create": {"description": "Create a new storage pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": true, "item_method": false, "no_auth_required": false,
    "accepts": [{"title": "pool_create", "type": "object", "additionalProperties": false, "properties": {
      "name": {"title": "name", "type": "string", "description": "The pool's name.", "_name_": "name", "_required_": true},
      "topology": {"title": "topology", "type": "string", "enum": ["STRIPE", "MIRROR"], "_name_": "topology", "_required_": true},
      "encryption": {"anyOf": [{"type": "boolean"}, {"type": "null"}], "title": "encryption", "default": false, "_name_": "encryption", "_required_": false},
      "tags": {"title": "tags", "type": "array", "items": [{"type": "string", "enum": ["a", "b"]}], "_name_": "tags", "_required_": false}},
      "_name_": "pool_create", "_required_": true}],
    "returns": [{"title": "PoolEntry", "type": "object", "properties": {
        "id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true},
        "name": {"title": "name", "type": "string", "_name_": "name", "_required_": true},
        "status": {"title": "status", "type": "string", "enum": ["ONLINE", "DEGRADED", ""], "_name_": "status", "_required_": true},
        "scan": {"anyOf": [{"title": "PoolScan", "type": "object", "properties": {"function": {"title": "function", "type": "string", "enum": ["SCRUB", "RESILVER"], "_name_": "function", "_required_": true}}}, {"type": "null"}], "title": "scan", "_name_": "scan", "_required_": true},
        "size": {"anyOf": [{"type": "integer"}, {"type": "null"}], "title": "size", "_name_": "size", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "pool.scrub.run": {"description": "Initiate a scrub of a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false,
    "accepts": [
      {"title": "name", "type": "string", "_name_": "name", "_required_": true},
      {"title": "threshold", "type": "integer", "default": 35, "_name_": "threshold", "_required_": false}],
    "returns": [{"title": "result", "type": "null", "_name_": "result", "_required_": true}]},
  "system.info": {"description": "Returns basic system information.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false,
    "accepts": [],
    "returns": [{"title": "result", "type": "object", "properties": {
      "hostname": {"title": "hostname", "type": "string", "_name_": "hostname", "_required_": true},
      "boottime": {"title": "boottime", "type": "string", "format": "date-time", "_name_": "boottime", "_required_": true},
      "state": {"title": "state", "type": "string", "enum": ["BOOTING", "READY"], "_name_": "state", "_required_": true},
      "license": {"anyOf": [{"type": "object", "additionalProperties": true}, {"type": "null"}], "title": "license", "_name_": "license", "_required_": true}},
      "_name_": "result", "_required_": true}]},
  "auth.login_ex": {"description": "Authenticate.", "roles": [], "filterable": false, "job": false, "item_method": false, "no_auth_required": true,
    "accepts": [{"title": "login_data", "discriminator": {"propertyName": "mechanism"}, "oneOf": [
      {"title": "AuthApiKeyPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "API_KEY_PLAIN", "enum": ["API_KEY_PLAIN"], "_name_": "mechanism", "_required_": true}, "api_key": {"title": "api_key", "type": "string", "_name_": "api_key", "_required_": true}}},
      {"title": "AuthPasswordPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "PASSWORD_PLAIN", "enum": ["PASSWORD_PLAIN"], "_name_": "mechanism", "_required_": true}, "password": {"title": "password", "type": "string", "_name_": "password", "_required_": true}}}],
      "_name_": "login_data", "_required_": true}],
    "returns": [{"title": "result", "type": "object", "properties": {"response_type": {"title": "response_type", "type": "string", "enum": ["SUCCESS", "AUTH_ERR"], "_name_": "response_type", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "gpu.one": {"description": "", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false, "accepts": [],
    "returns": [{"title": "GPU", "type": "object", "properties": {"vendor": {"title": "vendor", "type": "string", "_name_": "vendor", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "gpu.two": {"description": "", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false, "accepts": [],
    "returns": [{"title": "GPU", "type": "object", "properties": {"pci_slot": {"title": "pci_slot", "type": "string", "_name_": "pci_slot", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "disk.query": {"description": null, "roles": ["READONLY_ADMIN"], "filterable": true, "job": false, "item_method": false, "no_auth_required": false,
    "accepts": [
      {"title": "filters", "type": "array", "items": [{}], "default": [], "_name_": "filters", "_required_": false},
      {"title": "options", "type": "object", "additionalProperties": false, "properties": {
        "get": {"title": "get", "type": "boolean", "default": false, "_name_": "get", "_required_": false},
        "count": {"title": "count", "type": "boolean", "default": false, "_name_": "count", "_required_": false},
        "order_by": {"title": "order_by", "type": "array", "items": [{"type": "string"}], "_name_": "order_by", "_required_": false}},
        "_name_": "options", "_required_": false}],
    "returns": [{"title": "result", "anyOf": [{"type": "array", "items": [{"title": "DiskEntry", "type": "object", "properties": {"name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}}]}, {"type": "integer"}], "_name_": "result", "_required_": true}]},
  "disk.one": {"description": "", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false, "accepts": [],
    "returns": [{"title": "result", "type": "object", "properties": {"serial": {"title": "serial", "type": "string", "_name_": "serial", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "disk.two": {"description": "", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false, "accepts": [],
    "returns": [{"title": "result", "type": "object", "properties": {"serial": {"title": "serial", "type": "string", "_name_": "serial", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "reporting.get_data": {"description": "", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "item_method": false, "no_auth_required": false, "accepts": [],
    "returns": [{"title": "ReportingGetDataResponse", "type": "object", "properties": {"name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}, "_name_": "result", "_required_": true}]}
}`

// Each method is the Call operation of a path named for it, grouped by its namespace, with its parameters in order and its
// answer as the 200; a record titled as a class is one component however often it appears, one titled as a parameter is named
// for the method or the parameter, an enum for the record and the property it sits in, and a query's answer is its list.
func TestParse(t *testing.T) {
	t.Parallel()

	spec, notes, err := jsonrpc.Parse([]byte(description), "TrueNAS", "25.10.7")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Info.Title != "TrueNAS" || spec.Info.Version != "25.10.7" || len(spec.Paths) != 12 {
		t.Errorf("info = %+v, %d paths", spec.Info, len(spec.Paths))
	}
	for _, want := range []string{"two records are named GPU and differ, so the second is GPU2", "ReportingGetDataResponse is a name the package gives a method's answer, so the record is ReportingGetDataResponseModel"} {
		if !slices.Contains(notes, want) {
			t.Errorf("the notes lack %q: %q", want, notes)
		}
	}

	query := spec.Paths["pool.query"].Call
	if query == nil || query.OperationID != "pool.query" || query.Tags[0] != "pool" || query.Call == nil || !query.Call.Filterable || query.Call.Job || !slices.Equal(query.Call.Roles, []string{"POOL_READ", "READONLY_ADMIN"}) {
		t.Fatalf("pool.query = %+v", query)
	}
	if len(query.Parameters) != 2 || query.Parameters[0].Name != "filters" || query.Parameters[0].In != openapi.InCall || query.Parameters[0].Required || query.Parameters[1].Schema.Ref != openapi.SchemaRefPrefix+"QueryOptions" {
		t.Errorf("pool.query parameters = %+v", query.Parameters)
	}
	if answer := query.Responses["200"].Content["application/json"].Schema; answer.Type != openapi.TypeArray || answer.Items.Ref != openapi.SchemaRefPrefix+"PoolEntry" {
		t.Errorf("a query's answer = %+v", answer)
	}
	if create := spec.Paths["pool.create"].Call; !create.Call.Job || create.Parameters[0].Schema.Ref != openapi.SchemaRefPrefix+"PoolCreate" || !create.Parameters[0].Required || create.Description != "Create a new storage pool." {
		t.Errorf("pool.create = %+v", create)
	}
	if scrub := spec.Paths["pool.scrub.run"].Call; scrub.Tags[0] != "pool.scrub" || len(scrub.Responses["200"].Content) != 0 || scrub.Parameters[1].Required {
		t.Errorf("pool.scrub.run = %+v", scrub)
	}
	if login := spec.Paths["auth.login_ex"].Call; !login.Call.NoAuth || len(login.Parameters[0].Schema.OneOf) != 2 || login.Parameters[0].Schema.OneOf[0].Ref != openapi.SchemaRefPrefix+"AuthApiKeyPlain" {
		t.Errorf("auth.login_ex = %+v", login.Parameters[0].Schema)
	}
	// the records of a choice name themselves by the discriminator's property, each by the value fixed in it where the mapping gives none
	if disc := spec.Paths["auth.login_ex"].Call.Parameters[0].Schema.Discriminator; disc == nil || disc.PropertyName != "mechanism" || disc.Mapping["API_KEY_PLAIN"] != openapi.SchemaRefPrefix+"AuthApiKeyPlain" || disc.Mapping["PASSWORD_PLAIN"] != openapi.SchemaRefPrefix+"AuthPasswordPlain" {
		t.Errorf("auth.login_ex's discriminator = %+v", disc)
	}

	schemas := spec.Components.Schemas
	for name, props := range map[string][]string{
		"PoolEntry":                     {"id", "name", "scan", "size", "status"},
		"PoolScan":                      {"function"},
		"QueryOptions":                  {"order_by"},
		"PoolCreate":                    {"encryption", "name", "tags", "topology"},
		"GPU":                           {"vendor"},
		"GPU2":                          {"pci_slot"},
		"AuthApiKeyPlain":               {"api_key", "mechanism"},
		"ReportingGetDataResponseModel": {"name"},
	} {
		s := schemas[name]
		if s == nil {
			t.Errorf("no component %s among %s", name, strings.Join(openapi.SortedKeys(schemas), ", "))
			continue
		}
		if got := openapi.SortedKeys(s.Properties); !slices.Equal(got, props) {
			t.Errorf("%s has %v, want %v", name, got, props)
		}
	}
	for name, values := range map[string][]string{
		"PoolStatus":                    {"ONLINE", "DEGRADED", ""}, // met as PoolEntry's and PoolUpdate's status: one type, named by the words the two share
		"PoolScanFunction":              {"SCRUB", "RESILVER"},
		"PoolCreateTopology":            {"STRIPE", "MIRROR"},
		"PoolCreateTags":                {"a", "b"},
		"SystemInfoAnswerState":         {"BOOTING", "READY"},
		"AuthApiKeyPlainMechanism":      {"API_KEY_PLAIN"},
		"AuthLoginExAnswerResponseType": {"SUCCESS", "AUTH_ERR"},
	} {
		s := schemas[name]
		if s == nil || !s.IsEnum() {
			t.Errorf("no enum %s", name)
			continue
		}
		if got := s.EnumValues(); !slices.Equal(got, values) {
			t.Errorf("%s = %v, want %v", name, got, values)
		}
	}
	entry := schemas["PoolEntry"]
	if schemas["PoolEntryStatus"] != nil || schemas["PoolUpdateStatus"] != nil || entry.Properties["status"].Ref != openapi.SchemaRefPrefix+"PoolStatus" || schemas["PoolUpdate"].Properties["status"].Ref != openapi.SchemaRefPrefix+"PoolStatus" {
		t.Errorf("the status enum is not folded: %+v, %+v", entry.Properties["status"], schemas["PoolUpdate"].Properties["status"])
	}
	if !entry.Properties["scan"].Nullable || entry.Properties["scan"].Ref != openapi.SchemaRefPrefix+"PoolScan" || !entry.Properties["size"].Nullable || entry.Properties["size"].Type != openapi.TypeInteger {
		t.Errorf("null among the choices = scan %+v, size %+v", entry.Properties["scan"], entry.Properties["size"])
	}
	if !slices.Equal(entry.Required, []string{"id", "name", "scan", "size", "status"}) {
		t.Errorf("required = %v", entry.Required)
	}
	if create := schemas["PoolCreate"]; !create.Properties["encryption"].Nullable || create.Properties["encryption"].Type != openapi.TypeBoolean || create.Properties["tags"].Items.Ref != openapi.SchemaRefPrefix+"PoolCreateTags" {
		t.Errorf("pool_create = %+v", create.Properties)
	}
	// an answer declared inline is a component named as the importer would name it, so a record answered by several methods
	// is one model
	if answer := spec.Paths["system.info"].Call.Responses["200"].Content["application/json"].Schema; answer.Ref != openapi.SchemaRefPrefix+"SystemInfoAnswer" {
		t.Errorf("system.info's answer = %+v", answer)
	}
	info := schemas["SystemInfoAnswer"]
	if info == nil || info.Properties["boottime"].Format != "date-time" || info.Properties["state"].Ref != openapi.SchemaRefPrefix+"SystemInfoAnswerState" || !info.Properties["license"].Nullable {
		t.Errorf("system.info's answer = %+v", info)
	}
	// records of one shape under names that share their last words are one component named by those words, and
	// those that share only a word every answer ends in stay apart
	if schemas["QueryOptions"] == nil || schemas["PoolQueryOptions"] != nil || schemas["DiskQueryOptions"] != nil {
		t.Errorf("the query options were not folded: %s", strings.Join(openapi.SortedKeys(schemas), ", "))
	}
	if schemas["DiskOneAnswer"] == nil || schemas["DiskTwoAnswer"] == nil || schemas["Answer"] != nil {
		t.Errorf("two answers were folded under their tail word: %s", strings.Join(openapi.SortedKeys(schemas), ", "))
	}
}

// What the loader refuses: a description that is not an object of methods, one with none, a method that answers two schemas,
// and a parameter with no name or a name used twice.
func TestParseRefuses(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ doc, want string }{
		"not an object":       {`[1]`, "not an object of methods"},
		"no methods":          {`{}`, "names no methods"},
		"two answers":         {`{"a.b": {"accepts": [], "returns": [{"type": "null"}, {"type": "null"}]}}`, "a.b: answers 2 schemas, where a method answers one"},
		"a nameless param":    {`{"a.b": {"accepts": [{"type": "string"}], "returns": []}}`, "a.b: parameter 0 has no _name_"},
		"a name used twice":   {`{"a.b": {"accepts": [{"type": "string", "_name_": "x"}, {"type": "string", "_name_": "x"}], "returns": []}}`, "a.b: two parameters are named x"},
		"a name with a space": {`{"a b": {"accepts": [], "returns": []}}`, "a method's name has no space or slash"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := jsonrpc.Parse([]byte(tc.doc), "T", "1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse = %v, want containing %q", err, tc.want)
			}
		})
	}
}
