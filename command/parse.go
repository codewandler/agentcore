package command

import (
	"errors"
	"strings"
)

var (
	// ErrEmpty is returned when a command line is empty.
	ErrEmpty = errors.New("command: empty command")
	// ErrUnterminatedQuote is returned when a quoted argument is not closed.
	ErrUnterminatedQuote = errors.New("command: unterminated quoted string")
)

// Parse splits a slash command line into a command name and params.
//
// The leading slash is optional. Flags support --key, --key value, and
// --key=value forms. Single and double quoted strings are returned as one arg.
func Parse(line string) (string, Params, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", Params{}, ErrEmpty
	}
	line = strings.TrimPrefix(line, "/")
	if strings.TrimSpace(line) == "" {
		return "", Params{}, ErrEmpty
	}
	tokens, err := tokenize(line)
	if err != nil {
		return "", Params{}, err
	}
	if len(tokens) == 0 {
		return "", Params{}, ErrEmpty
	}

	name := tokens[0]
	rest := tokens[1:]
	params := Params{
		Raw:   strings.Join(rest, " "),
		Args:  []string{},
		Flags: map[string]string{},
	}
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if !strings.HasPrefix(tok, "--") || tok == "--" {
			params.Args = append(params.Args, tok)
			continue
		}
		key := strings.TrimPrefix(tok, "--")
		if idx := strings.IndexByte(key, '='); idx >= 0 {
			params.Flags[key[:idx]] = key[idx+1:]
			continue
		}
		if i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "--") {
			params.Flags[key] = rest[i+1]
			i++
			continue
		}
		params.Flags[key] = "true"
	}
	return name, params, nil
}

func tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	var quote byte
	escaped := false

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tokens = append(tokens, cur.String())
		cur.Reset()
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	if escaped {
		cur.WriteByte('\\')
	}
	if quote != 0 {
		return nil, ErrUnterminatedQuote
	}
	flush()
	return tokens, nil
}
