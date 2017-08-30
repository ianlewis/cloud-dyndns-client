// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io/ioutil"
	"os"
	"strings"
)

// Writes out the version
func main() {
	out, _ := os.Create("version.go")
	f, _ := os.Open("VERSION")
	b, _ := ioutil.ReadAll(f)

	out.WriteString("package main\n\n")
	out.WriteString("const VERSION = `")
	out.WriteString(strings.Trim(string(b), " \n\r"))
	out.WriteString("`\n")
}
