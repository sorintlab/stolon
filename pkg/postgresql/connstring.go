// Copyright 2015 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package postgresql

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"unicode"
)

// This is based on github.com/lib/pq

type connParams map[string]string

func (p connParams) Set(k, v string) {
	p[k] = v
}

func (p connParams) Get(k string) (v string) {
	return p[k]
}

func (p connParams) Isset(k string) bool {
	_, ok := p[k]
	return ok
}

func (p connParams) Equals(cp connParams) bool {
	return reflect.DeepEqual(p, cp)
}

// scanner implements a tokenizer for libpq-style option strings.
type scanner struct {
	s []rune
	i int
}

// newScanner returns a new scanner initialized with the option string s.
func newScanner(s string) *scanner {
	return &scanner{[]rune(s), 0}
}

// Next returns the next rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) Next() (rune, bool) {
	if s.i >= len(s.s) {
		return 0, false
	}
	r := s.s[s.i]
	s.i++
	return r, true
}

// SkipSpaces returns the next non-whitespace rune.
// It returns 0, false if the end of the text has been reached.
func (s *scanner) SkipSpaces() (rune, bool) {
	r, ok := s.Next()
	for unicode.IsSpace(r) && ok {
		r, ok = s.Next()
	}
	return r, ok
}

// ParseConnString parses the options from name and adds them to the values.
//
// The parsing code is based on conninfo_parse from libpq's fe-connect.c
func ParseConnString(name string) (connParams, error) {
	p := make(connParams)
	s := newScanner(name)

	for {
		var (
			keyRunes, valRunes []rune
			r                  rune
			ok                 bool
		)

		if r, ok = s.SkipSpaces(); !ok {
			break
		}

		// Scan the key
		for !unicode.IsSpace(r) && r != '=' {
			keyRunes = append(keyRunes, r)
			if r, ok = s.Next(); !ok {
				break
			}
		}

		// Skip any whitespace if we're not at the = yet
		if r != '=' {
			r, ok = s.SkipSpaces()
		}

		// The current character should be =
		if r != '=' || !ok {
			return nil, fmt.Errorf(`missing "=" after %q in connection info string"`, string(keyRunes))
		}

		// Skip any whitespace after the =
		if r, ok = s.SkipSpaces(); !ok {
			// If we reach the end here, the last value is just an empty string as per libpq.
			p.Set(string(keyRunes), "")
			break
		}

		if r != '\'' {
			for !unicode.IsSpace(r) {
				if r == '\\' {
					if r, ok = s.Next(); !ok {
						return nil, fmt.Errorf(`missing character after backslash`)
					}
				}
				valRunes = append(valRunes, r)

				if r, ok = s.Next(); !ok {
					break
				}
			}
		} else {
		quote:
			for {
				if r, ok = s.Next(); !ok {
					return nil, fmt.Errorf(`unterminated quoted string literal in connection string`)
				}
				switch r {
				case '\'':
					break quote
				case '\\':
					r, _ = s.Next()
					fallthrough
				default:
					valRunes = append(valRunes, r)
				}
			}
		}

		p.Set(string(keyRunes), string(valRunes))
	}

	return p, nil
}

// URLToConnParams creates the connParams from the url.
func URLToConnParams(urlStr string) (connParams, error) {
	p := make(connParams)
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "postgres" {
		return nil, fmt.Errorf("invalid connection protocol: %s", u.Scheme)
	}

	if u.User != nil {
		v := u.User.Username()
		p.Set("user", v)
		v, _ = u.User.Password()
		p.Set("password", v)
	}

	i := strings.Index(u.Host, ":")
	if i < 0 {
		p.Set("host", u.Host)
	} else {
		p.Set("host", u.Host[:i])
		p.Set("port", u.Host[i+1:])
	}

	if u.Path != "" {
		p.Set("dbname", u.Path[1:])
	}

	q := u.Query()
	for k := range q {
		p.Set(k, q.Get(k))
	}

	return p, nil
}

func (p connParams) ConnString() string {
	var kvs []string
	escaper := strings.NewReplacer(` `, `\ `, `'`, `\'`, `\`, `\\`)
	for k, v := range p {
		if v != "" {
			kvs = append(kvs, k+"="+escaper.Replace(v))
		}
	}
	return strings.Join(kvs, " ")
}
