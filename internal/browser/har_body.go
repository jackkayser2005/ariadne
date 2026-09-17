package browser

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

type harPostData struct {
	Params         []harName `json:"params"`
	Text           *string   `json:"text"`
	MimeType       string    `json:"mimeType"`
	Encoding       string    `json:"encoding"`
	ExportEncoding string    `json:"_encoding"`
}

type harBody struct {
	values    []string
	channel   string
	inspected bool
	gap       bool
}

// inspectHARBody interprets only bounded textual export representations. It
// never makes a claim about their on-wire encoding or delivery.
func inspectHARBody(post *harPostData, bodySize *int64) harBody {
	gap := harBody{gap: true}
	if post == nil {
		if bodySize != nil && *bodySize > 0 {
			return gap
		}
		return harBody{}
	}
	if post.Text == nil || len(*post.Text) > 64<<10 || post.Encoding != "" || post.ExportEncoding != "" {
		return gap
	}
	kind, params, err := mime.ParseMediaType(post.MimeType)
	if err != nil || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
		return gap
	}
	result := harBody{inspected: true}
	if kind == "application/x-www-form-urlencoded" {
		fields, err := url.ParseQuery(*post.Text)
		if err != nil || len(fields) > 1024 {
			return gap
		}
		result.channel = "Form body value"
		for _, values := range fields {
			result.values = append(result.values, values...)
		}
		return result
	}
	if kind != "application/json" && !(strings.HasPrefix(kind, "application/") && strings.HasSuffix(kind, "+json")) {
		return gap
	}
	data := []byte(*post.Text)
	// Limit nesting before the duplicate-key check and recursive string walk.
	decoder := json.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return gap
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
				if depth > 32 {
					return gap
				}
			case '}', ']':
				depth--
			}
		}
	}
	if !json.Valid(data) || jsoncheck.RejectDuplicateKeys(data) != nil {
		return gap
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return gap
	}
	result.channel = "JSON body string"
	var visit func(any)
	visit = func(v any) {
		switch item := v.(type) {
		case string:
			result.values = append(result.values, item)
		case []any:
			for _, child := range item {
				visit(child)
			}
		case map[string]any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return result
}
