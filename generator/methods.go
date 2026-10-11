package generator

import (
	"fmt"
	"go/token"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/katbyte/pandorest/definitions"
)

// methodFile renders one operation: its response struct, its options struct
// when it has options, the method, and the Complete pager for a list.
func (g *gen) methodFile(o *definitions.Operation) string {
	responseType := o.Name + "OperationResponse"
	optionsType := o.Name + "Options"
	g.declare(responseType, "operation "+o.Name)

	var b strings.Builder
	g.writeResponse(&b, o, responseType)
	if len(o.Options) > 0 {
		g.declare(optionsType, "operation "+o.Name)
		g.writeOptions(&b, o, optionsType)
	}
	g.writeMethod(&b, o, responseType, optionsType)
	if o.Pageable != nil {
		g.writeCompleteResult(&b, o)
		g.writeCompleteMethod(&b, o, optionsType)
	}

	return g.fileHeader(b.String())
}

// operationFile renders, in the types package of the grouped layout, what a
// caller names of one operation: its response struct, its options struct and
// the result type of its pager.
func (g *gen) operationFile(o *definitions.Operation) string {
	responseType := o.Name + "OperationResponse"
	optionsType := o.Name + "Options"
	g.declare(responseType, "operation "+o.Name)

	var b strings.Builder
	g.writeResponse(&b, o, responseType)
	if len(o.Options) > 0 {
		g.declare(optionsType, "operation "+o.Name)
		g.writeOptions(&b, o, optionsType)
	}
	if o.Pageable != nil {
		g.writeCompleteResult(&b, o)
	}

	return g.fileHeader(b.String())
}

// groupMethodFile renders, in a tag's package of the grouped layout, one
// operation's method and, for a list, its pager, naming the types as the
// types package's.
func (g *gen) groupMethodFile(o *definitions.Operation) string {
	var b strings.Builder
	g.writeMethod(&b, o, g.typeName(o.Name+"OperationResponse"), g.typeName(o.Name+"Options"))
	if o.Pageable != nil {
		g.writeCompleteMethod(&b, o, g.typeName(o.Name+"Options"))
	}

	return g.fileHeader(b.String())
}

// responseModel is how the response struct holds the model: pointers for a
// struct, a constant or a primitive, the value for slices, maps and raw JSON.
func (g *gen) responseModel(o *definitions.Operation) (typ string, pointer bool) {
	if o.Response == nil || o.Response.Type.Type == definitions.RawFile {
		return "", false
	}
	t := o.Response.Type
	switch t.Type {
	case definitions.List, definitions.Dictionary, definitions.RawObject, definitions.Any:
		return g.goType(t), false
	case definitions.Reference:
		if m := g.models[t.ReferenceName]; m != nil && len(m.Union) > 0 {
			return t.ReferenceName, false
		}
	default:
	}

	return "*" + g.goType(t), true
}

func (g *gen) writeResponse(b *strings.Builder, o *definitions.Operation, responseType string) {
	fmt.Fprintf(b, "// %s is the result of %s.\n", responseType, o.Name)
	if o.Response != nil && o.Response.Type.Type == definitions.RawFile {
		fmt.Fprintf(b, "//\n// The operation answers %s, left unread in HttpResponse.Body, which\n// the caller must close.\n", o.Response.ContentType)
	}
	if o.Response.Text() {
		fmt.Fprintf(b, "//\n// The operation answers text (%s), which Model holds as it was sent.\n", o.Response.ContentType)
	}
	fmt.Fprintf(b, "type %s struct {\n\tHttpResponse *http.Response\n", responseType)
	if typ, _ := g.responseModel(o); typ != "" {
		fmt.Fprintf(b, "\tModel %s\n", typ)
	}
	b.WriteString("}\n\n")
}

// optionType is how the options struct holds an option: a pointer for a
// boolean or a number, so false and 0 can be sent (a series' specials are
// season 0, a backdrop is image 0), the plain type for the rest, whose zero
// value is not sent.
func (g *gen) optionType(t definitions.TypeRef) string {
	switch t.Type {
	case definitions.Boolean, definitions.Integer, definitions.Integer64, definitions.Float, definitions.Double:
		return "*" + g.goType(t)
	default:
		return g.goType(t)
	}
}

func (g *gen) writeOptions(b *strings.Builder, o *definitions.Operation, optionsType string) {
	fmt.Fprintf(b, "// %s holds the query and header parameters of %s.\n", optionsType, o.Name)
	fmt.Fprintf(b, "type %s struct {\n", optionsType)
	for i, opt := range o.Options {
		doc := opt.Description
		if opt.Deprecated {
			doc = strings.TrimSpace(doc + "\n\nDeprecated: the document marks this parameter deprecated.")
		}
		if i > 0 && doc != "" {
			b.WriteString("\n")
		}
		b.WriteString(comment("\t", doc))
		fmt.Fprintf(b, "\t%s %s\n", opt.Field, g.optionType(opt.Type))
	}
	b.WriteString("}\n\n")

	for _, in := range []struct{ where, method, typ string }{
		{definitions.InHeader, "ToHeaders", "Headers"},
		{definitions.InQuery, "ToQuery", "QueryParams"},
	} {
		fmt.Fprintf(b, "// %s returns the %s the options set.\n", in.method, strings.ToLower(in.where)+" parameters")
		fmt.Fprintf(b, "func (o %s) %s() *client.%s {\n\tout := client.%s{}\n", optionsType, in.method, in.typ, in.typ)
		for _, opt := range o.Options {
			if opt.In == in.where {
				b.WriteString(appendOption(opt))
			}
		}
		b.WriteString("\treturn &out\n}\n\n")
	}
}

// appendOption renders the statement that adds one option when it is set. A
// list goes comma-separated or as one parameter per element, as the
// document says.
func appendOption(opt definitions.Option) string {
	field := "o." + opt.Field
	var cond, value string
	switch t := opt.Type; t.Type {
	case definitions.Boolean:
		cond, value = field+" != nil", "strconv.FormatBool(*"+field+")"
	case definitions.Integer:
		cond, value = field+" != nil", "strconv.Itoa(*"+field+")"
	case definitions.Integer64:
		cond, value = field+" != nil", "strconv.FormatInt(*"+field+", 10)"
	case definitions.Float:
		cond, value = field+" != nil", "strconv.FormatFloat(float64(*"+field+"), 'f', -1, 32)"
	case definitions.Double:
		cond, value = field+" != nil", "strconv.FormatFloat(*"+field+", 'f', -1, 64)"
	case definitions.List:
		if !opt.CommaSeparated {
			item := "v"
			switch t.NestedItem.Type {
			case definitions.Integer:
				item = "strconv.Itoa(v)"
			case definitions.Integer64:
				item = "strconv.FormatInt(v, 10)"
			case definitions.Reference:
				item = "string(v)"
			default:
			}
			return fmt.Sprintf("\tfor _, v := range %s {\n\t\tout.Append(%q, %s)\n\t}\n", field, opt.Name, item)
		}
		cond, value = "len("+field+") > 0", "client.CSV("+field+")"
	case definitions.Dictionary:
		if opt.DeepObject {
			return fmt.Sprintf("\tfor _, kv := range client.DeepObject(%q, %s) {\n\t\tout.Append(kv[0], kv[1])\n\t}\n", opt.Name, field)
		}
		cond, value = "len("+field+") > 0", "client.JSONObject("+field+")"
	case definitions.Reference:
		cond, value = field+` != ""`, "string("+field+")"
	default:
		cond, value = field+` != ""`, field
	}

	return fmt.Sprintf("\tif %s {\n\t\tout.Append(%q, %s)\n\t}\n", cond, opt.Name, value)
}

// pathExpression renders the path with each placeholder filled from its
// escaped argument.
func pathExpression(o *definitions.Operation) string {
	if len(o.PathParameters) == 0 {
		return fmt.Sprintf("%q", o.Path)
	}
	byName := map[string]definitions.PathParameter{}
	for _, p := range o.PathParameters {
		byName[p.Name] = p
	}
	var format strings.Builder
	var args []string
	rest := o.Path
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		closeIdx := strings.IndexByte(rest, '}')
		if open < 0 || closeIdx < open {
			format.WriteString(strings.ReplaceAll(rest, "%", "%%"))
			break
		}
		format.WriteString(strings.ReplaceAll(rest[:open], "%", "%%"))
		p := byName[rest[open+1:closeIdx]]
		switch p.Type.Type {
		case definitions.Integer, definitions.Integer64:
			format.WriteString("%d")
			args = append(args, p.Argument)
		case definitions.Double:
			format.WriteString("%s")
			args = append(args, "strconv.FormatFloat("+p.Argument+", 'f', -1, 64)")
		case definitions.Reference:
			format.WriteString("%s")
			args = append(args, "url.PathEscape(string("+p.Argument+"))")
		default:
			format.WriteString("%s")
			args = append(args, "url.PathEscape("+p.Argument+")")
		}
		rest = rest[closeIdx+1:]
	}

	return fmt.Sprintf("fmt.Sprintf(%q, %s)", format.String(), strings.Join(args, ", "))
}

// arguments renders the method's parameters after ctx, and the call that
// passes them on.
func (g *gen) arguments(o *definitions.Operation, optionsType string) (params, call []string) {
	for _, p := range o.PathParameters {
		params = append(params, p.Argument+" "+g.goType(p.Type))
		call = append(call, p.Argument)
	}
	if o.Request != nil {
		if o.Request.Type.Type == definitions.RawFile {
			params = append(params, "input io.Reader", "contentType string")
			call = append(call, "input", "contentType")
		} else {
			params = append(params, "input "+g.goType(o.Request.Type))
			call = append(call, "input")
		}
	}
	switch {
	case g.inline(o):
		opt := &o.Options[0]
		if opt.Required {
			params = append(params, g.inlineArgument(o)+" "+g.goType(opt.Type))
		} else {
			params = append(params, g.inlineArgument(o)+" ..."+g.inlineType(opt))
		}
		call = append(call, "option")
	case g.optional(o):
		params = append(params, "options ..."+optionsType)
		call = append(call, "option")
	case len(o.Options) > 0:
		params = append(params, "options "+optionsType)
		call = append(call, "options")
	}

	return params, call
}

// inline reports whether an operation's one option is taken as an argument in
// place of the struct (Options.InlineSingleOptions): GetMovieLookup(ctx, term). A
// list's pager sets the page options itself, so a list keeps its struct.
func (g *gen) inline(o *definitions.Operation) bool {
	return g.opts.InlineSingleOptions && len(o.Options) == 1 && o.Pageable == nil
}

// inlineType is the argument's type: the option's own, without the pointer
// the struct holds a boolean or a number by, since an argument left out is
// what the pointer's nil said; an optional list is taken as its elements,
// a required one as the slice.
func (g *gen) inlineType(opt *definitions.Option) string {
	if opt.Type.Type == definitions.List {
		return g.goType(*opt.Type.NestedItem)
	}

	return g.goType(opt.Type)
}

// localNames are the names a method body uses and the packages it imports,
// which an inlined option's argument keeps clear of.
var localNames = map[string]bool{"ctx": true, "input": true, "contentType": true, "opts": true, "req": true, "resp": true, "result": true, "err": true, "option": true, "options": true, "model": true, "c": true, "client": true, "context": true, "fmt": true, "http": true, "io": true, "url": true, "strconv": true, "strings": true, "json": true, "errors": true}

// inlineArgument names the argument of an inlined option: the field's name
// lower-cased at the front (Term: term), kept clear of the body's own names,
// the packages it uses, Go's keywords and the path parameters by a suffix
// (Type: typeOption, Url: urlOption).
func (g *gen) inlineArgument(o *definitions.Operation) string {
	field := o.Options[0].Field
	first, size := utf8.DecodeRuneInString(field)
	name := string(unicode.ToLower(first)) + field[size:]
	if localNames[name] || name == g.qual || name == g.svc.Package || token.IsKeyword(name) || slices.ContainsFunc(o.PathParameters, func(p definitions.PathParameter) bool { return p.Argument == name }) {
		name += "Option"
	}

	return name
}

// optional reports whether an operation's options may all be left out, so
// that a call can leave the struct out altogether: GetSessions(ctx).
func (g *gen) optional(o *definitions.Operation) bool {
	return len(o.Options) > 0 && !g.inline(o) && !slices.ContainsFunc(o.Options, func(p definitions.Option) bool { return p.Required })
}

// optionsVar is the name the body reads the options by: the one struct a
// variadic call was given, the struct built from an inlined argument, or the
// parameter itself.
func (g *gen) optionsVar(o *definitions.Operation) string {
	if g.optional(o) || g.inline(o) {
		return "option"
	}

	return "options"
}

// writeOptionsPrelude builds the options the body sends: the one struct a
// variadic call was given, or the struct an inlined argument fills, refusing
// more than one before any request is made.
func (g *gen) writeOptionsPrelude(b *strings.Builder, o *definitions.Operation, method, optionsType string) {
	switch {
	case g.inline(o):
		opt := &o.Options[0]
		arg := g.inlineArgument(o)
		fmt.Fprintf(b, "\tvar option %s\n", optionsType)
		switch {
		case opt.Required:
			fmt.Fprintf(b, "\toption.%s = %s\n\n", opt.Field, held(opt, arg))
		case opt.Type.Type == definitions.List:
			fmt.Fprintf(b, "\tif len(%s) > 0 {\n\t\toption.%s = %s\n\t}\n\n", arg, opt.Field, arg)
		default:
			fmt.Fprintf(b, "\tif len(%[1]s) > 1 {\n\t\terr = fmt.Errorf(\"%[2]s takes one %[1]s at most, not %%d\", len(%[1]s))\n\t\treturn\n\t}\n", arg, method)
			fmt.Fprintf(b, "\tif len(%s) == 1 {\n\t\toption.%s = %s\n\t}\n\n", arg, opt.Field, held(opt, arg+"[0]"))
		}
	case g.optional(o):
		fmt.Fprintf(b, "\tif len(options) > 1 {\n\t\terr = fmt.Errorf(\"%s takes one options struct at most, not %%d\", len(options))\n\t\treturn\n\t}\n", method)
		fmt.Fprintf(b, "\tvar option %s\n\tif len(options) == 1 {\n\t\toption = options[0]\n\t}\n\n", optionsType)
	}
}

// held is the expression the options struct holds an argument's value by: its
// address for a boolean or a number, the value for the rest (see optionType).
func held(opt *definitions.Option, expr string) string {
	switch opt.Type.Type {
	case definitions.Boolean, definitions.Integer, definitions.Integer64, definitions.Float, definitions.Double:
		return "&" + expr
	default:
		return expr
	}
}

var httpMethods = map[string]string{
	"GET": "http.MethodGet", "POST": "http.MethodPost", "PUT": "http.MethodPut", "DELETE": "http.MethodDelete", "PATCH": "http.MethodPatch",
}

var statusNames = map[int]string{
	http.StatusOK: "http.StatusOK", http.StatusCreated: "http.StatusCreated", http.StatusAccepted: "http.StatusAccepted",
	http.StatusNoContent: "http.StatusNoContent", http.StatusPartialContent: "http.StatusPartialContent",
	http.StatusBadRequest:       "http.StatusBadRequest",
	http.StatusMovedPermanently: "http.StatusMovedPermanently", http.StatusFound: "http.StatusFound", http.StatusSeeOther: "http.StatusSeeOther",
	http.StatusNotModified: "http.StatusNotModified", http.StatusTemporaryRedirect: "http.StatusTemporaryRedirect", http.StatusPermanentRedirect: "http.StatusPermanentRedirect",
}

func (g *gen) writeMethod(b *strings.Builder, o *definitions.Operation, responseType, optionsType string) {
	g.declare("Client."+o.Name, "operation "+o.Name)
	params, _ := g.arguments(o, optionsType)

	fmt.Fprintf(b, "// %s calls %s %s.", o.Name, o.Method, o.Path)
	if s := firstSentence(o.Description); s != "" {
		b.WriteString(" " + s)
	}
	b.WriteString("\n")
	if o.Request != nil && o.Request.Type.Type == definitions.RawFile {
		if strings.Contains(o.Request.ContentType, "*") {
			fmt.Fprintf(b, "//\n// input is sent as contentType, which must name a concrete type: the\n// operation takes %s.\n", o.Request.ContentType)
		} else {
			fmt.Fprintf(b, "//\n// input is sent as contentType, or as %s when that is empty.\n", o.Request.ContentType)
		}
	}
	if o.Deprecated {
		b.WriteString("//\n// Deprecated: the document marks this operation deprecated.\n")
	}
	fmt.Fprintf(b, "func (c Client) %s(%s) (result %s, err error) {\n",
		o.Name, strings.Join(append([]string{"ctx context.Context"}, params...), ", "), responseType)
	g.writeOptionsPrelude(b, o, o.Name, optionsType)

	b.WriteString("\topts := client.RequestOptions{\n")
	if asksFor := o.Response.AsksFor(); asksFor != "" {
		// a server may choose its answer by what it is asked for, so a call asks for what its document says it answers
		fmt.Fprintf(b, "\t\tAccept: %q,\n", asksFor)
	}
	if o.Request != nil {
		fmt.Fprintf(b, "\t\tContentType: %q,\n", o.Request.ContentType)
	}
	b.WriteString("\t\tExpectedStatusCodes: []int{\n")
	for _, code := range o.ExpectedStatusCodes {
		name, ok := statusNames[code]
		if !ok {
			name = strconv.Itoa(code)
		}
		fmt.Fprintf(b, "\t\t\t%s,\n", name)
	}
	b.WriteString("\t\t},\n")
	fmt.Fprintf(b, "\t\tHTTPMethod: %s,\n", httpMethods[o.Method])
	if len(o.Options) > 0 {
		fmt.Fprintf(b, "\t\tOptionsObject: %s,\n", g.optionsVar(o))
	}
	fmt.Fprintf(b, "\t\tPath: %s,\n", pathExpression(o))
	if o.Response != nil && o.Response.Type.Type == definitions.RawFile {
		b.WriteString("\t\tStreamResponse: true,\n")
	}
	b.WriteString("\t}\n\n")

	b.WriteString("\treq, err := c.Client.NewRequest(ctx, opts)\n\tif err != nil {\n\t\treturn\n\t}\n\n")
	if o.Request != nil {
		if o.Request.Type.Type == definitions.RawFile {
			b.WriteString("\tif err = req.SetBody(input, contentType); err != nil {\n\t\treturn\n\t}\n\n")
		} else {
			b.WriteString("\tif err = req.Marshal(input); err != nil {\n\t\treturn\n\t}\n\n")
		}
	}

	b.WriteString("\tvar resp *client.Response\n\tresp, err = req.Execute(ctx)\n")
	b.WriteString("\tif resp != nil {\n\t\tresult.HttpResponse = resp.Response\n\t}\n")
	b.WriteString("\tif err != nil {\n\t\treturn\n\t}\n\n")

	if typ, pointer := g.responseModel(o); typ != "" {
		if slices.Contains(o.ExpectedStatusCodes, http.StatusNoContent) {
			// a 204 is a null result: no model
			b.WriteString("\tif resp.StatusCode == http.StatusNoContent {\n\t\treturn\n\t}\n\n")
		}
		switch {
		case o.Response.Text():
			// text is held as it was sent, whatever the server calls it
			b.WriteString("\tvar model string\n\tresult.Model = &model\n")
			b.WriteString("\tif err = resp.Text(result.Model); err != nil {\n\t\treturn\n\t}\n\n")
		case pointer:
			fmt.Fprintf(b, "\tvar model %s\n\tresult.Model = &model\n", strings.TrimPrefix(typ, "*"))
			b.WriteString("\tif err = resp.Unmarshal(result.Model); err != nil {\n\t\treturn\n\t}\n\n")
		default:
			b.WriteString("\tif err = resp.Unmarshal(&result.Model); err != nil {\n\t\treturn\n\t}\n\n")
		}
	}
	b.WriteString("\treturn\n}\n")
}

// writeCompleteResult declares the result of a list's pager.
func (g *gen) writeCompleteResult(b *strings.Builder, o *definitions.Operation) {
	resultType := o.Name + "CompleteResult"
	g.declare(resultType, "operation "+o.Name)

	fmt.Fprintf(b, "\n// %s is every result %sComplete loaded.\n", resultType, o.Name)
	fmt.Fprintf(b, "type %s struct {\n\tLatestHttpResponse *http.Response\n\tItems []%s\n}\n\n", resultType, g.goType(o.Pageable.ItemType))
}

// writeCompleteMethod writes a list's pager.
func (g *gen) writeCompleteMethod(b *strings.Builder, o *definitions.Operation, optionsType string) {
	p := o.Pageable
	method := o.Name + "Complete"
	resultType := g.typeName(o.Name + "CompleteResult")
	g.declare("Client."+method, "operation "+o.Name)
	params, call := g.arguments(o, optionsType)

	switch {
	case p.ByPage && p.LimitOption == "":
		fmt.Fprintf(b, "// %s calls %s page by page, from the\n", method, o.Name)
		fmt.Fprintf(b, "// page options.%s names (the first when unset), until every result is loaded.\n", p.StartIndexOption)
		b.WriteString("// The page size is the server's: a page shorter than the first is the last.\n")
	case p.ByPage:
		fmt.Fprintf(b, "// %s calls %s page by page, from the\n", method, o.Name)
		fmt.Fprintf(b, "// page options.%s names (the first when unset), until every result is loaded.\n", p.StartIndexOption)
		fmt.Fprintf(b, "// options.%s is the page size, client.DefaultPageSize when unset.\n", p.LimitOption)
	default:
		fmt.Fprintf(b, "// %s calls %s page by page from options.%s until every\n", method, o.Name, p.StartIndexOption)
		fmt.Fprintf(b, "// result is loaded. options.%s is the page size, client.DefaultPageSize when\n// unset.\n", p.LimitOption)
	}
	fmt.Fprintf(b, "func (c Client) %s(%s) (result %s, err error) {\n", method, strings.Join(append([]string{"ctx context.Context"}, params...), ", "), resultType)
	g.writeOptionsPrelude(b, o, method, optionsType)
	ov := g.optionsVar(o)
	if p.LimitOption == "" {
		// the server's page size, learnt from the first page
		b.WriteString("\tlimit := 0\n")
	} else {
		fmt.Fprintf(b, "\tlimit := client.DefaultPageSize\n\tif %[2]s.%[1]s != nil && *%[2]s.%[1]s > 0 {\n\t\tlimit = *%[2]s.%[1]s\n\t}\n", p.LimitOption, ov)
		fmt.Fprintf(b, "\t%s.%s = &limit\n", ov, p.LimitOption)
	}
	if p.ByPage {
		fmt.Fprintf(b, "\tnumber := 1\n\tif %[2]s.%[1]s != nil && *%[2]s.%[1]s > 0 {\n\t\tnumber = *%[2]s.%[1]s\n\t}\n", p.StartIndexOption, ov)
		fmt.Fprintf(b, "\t%s.%s = &number\n", ov, p.StartIndexOption)
	} else {
		fmt.Fprintf(b, "\tstart := 0\n\tif %[2]s.%[1]s != nil {\n\t\tstart = *%[2]s.%[1]s\n\t}\n", p.StartIndexOption, ov)
	}
	b.WriteString("\tfor {\n")
	fmt.Fprintf(b, "\t\tvar page %s\n", g.typeName(o.Name+"OperationResponse"))
	fmt.Fprintf(b, "\t\tpage, err = c.%s(%s)\n", o.Name, strings.Join(append([]string{"ctx"}, call...), ", "))
	b.WriteString("\t\tresult.LatestHttpResponse = page.HttpResponse\n")
	b.WriteString("\t\tif err != nil {\n\t\t\terr = fmt.Errorf(\"loading results: %w\", err)\n\t\t\treturn\n\t\t}\n")
	fmt.Fprintf(b, "\t\tif page.Model == nil || len(page.Model.%s) == 0 {\n\t\t\treturn\n\t\t}\n", p.ItemsField)
	fmt.Fprintf(b, "\t\tresult.Items = append(result.Items, page.Model.%s...)\n", p.ItemsField)
	switch {
	case p.ByPage && p.LimitOption == "":
		b.WriteString("\t\t// the first page says how long a page is; a shorter one is the last, and\n")
		b.WriteString("\t\t// so is the page that reaches the total, when the server reports one\n")
		fmt.Fprintf(b, "\t\tif limit == 0 {\n\t\t\tlimit = len(page.Model.%s)\n\t\t}\n", p.ItemsField)
		fmt.Fprintf(b, "\t\tif len(page.Model.%[1]s) < limit || page.Model.%[2]s > 0 && len(result.Items) >= page.Model.%[2]s {\n\t\t\treturn\n\t\t}\n", p.ItemsField, p.TotalField)
		b.WriteString("\t\tnumber++\n")
	case p.ByPage:
		b.WriteString("\t\t// a short page is the last; so is the page that reaches the total, when\n")
		b.WriteString("\t\t// the server reports one\n")
		fmt.Fprintf(b, "\t\tif len(page.Model.%[1]s) < limit || page.Model.%[2]s > 0 && number*limit >= page.Model.%[2]s {\n\t\t\treturn\n\t\t}\n", p.ItemsField, p.TotalField)
		// options holds a pointer to number, so the next call asks for the next page
		b.WriteString("\t\tnumber++\n")
	default:
		fmt.Fprintf(b, "\t\tstart += len(page.Model.%s)\n\t\t%s.%s = &start\n", p.ItemsField, ov, p.StartIndexOption)
		b.WriteString("\t\t// a short page is the last; so is reaching the total, when the server\n")
		b.WriteString("\t\t// reports one (a list may report 0 whatever it holds)\n")
		fmt.Fprintf(b, "\t\tif len(page.Model.%[1]s) < limit || page.Model.%[2]s > 0 && start >= page.Model.%[2]s {\n\t\t\treturn\n\t\t}\n", p.ItemsField, p.TotalField)
	}
	b.WriteString("\t}\n}\n")
}
