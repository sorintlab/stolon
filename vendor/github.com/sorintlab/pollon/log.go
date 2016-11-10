// Copyright 2016 Sorint.lab
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

package pollon

// Use a nop logger by default.
// Access is not mutex-protected: do not modify except in init()
// functions.
var log Logger = &nopLogger{}

// Logger mimics golang's standard Logger as an interface. Only Print
// functions are needed.
type Logger interface {
	Print(args ...interface{})
	Printf(format string, args ...interface{})
	Println(args ...interface{})
}

// SetLogger sets the logger used in this package. It should be called
// only from package user init() functions.
func SetLogger(l Logger) {
	log = l
}

type nopLogger struct{}

func (l *nopLogger) Print(args ...interface{})                 {}
func (l *nopLogger) Printf(format string, args ...interface{}) {}
func (l *nopLogger) Println(args ...interface{})               {}
