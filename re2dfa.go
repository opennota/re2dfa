// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
// Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program.  If not, see <http://www.gnu.org/licenses/>.

// Transform regular expressions into finite state machines.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/opennota/re2dfa/codegen"
	"github.com/opennota/re2dfa/dfa"
	"github.com/opennota/re2dfa/nfa"
)

func main() {
	log.SetFlags(0)

	output := flag.String("o", "", "Output to file")
	flag.Usage = func() {
		fmt.Println(`Usage: re2dfa [options] regexp package.function string|[]byte

Options:
    -o FILE    Output to FILE instead of standard output

EXAMPLE: re2dfa ^a+$ main.matchAPlus string`)
	}
	flag.Parse()
	if len(flag.Args()) != 3 {
		flag.Usage()
		os.Exit(1)
	}

	expr := flag.Arg(0)
	_, err := regexp.Compile(expr)
	if err != nil {
		log.Fatal(fmt.Sprintf("invalid regexp: %q", expr))
	}

	pkgfun := strings.Split(flag.Arg(1), ".")
	if len(pkgfun) != 2 {
		flag.Usage()
		os.Exit(1)
	}
	pkg := pkgfun[0]
	fun := pkgfun[1]
	typ := flag.Arg(2)

	if !(typ == "string" || typ == "[]byte") {
		flag.Usage()
		os.Exit(1)
	}

	nfanode, err := nfa.New(expr)
	if err != nil {
		log.Fatal(err)
	}

	node := dfa.NewFromNFA(nfanode)
	source := codegen.GoGenerate(node, pkg, fun, typ)
	if *output == "" {
		fmt.Println(source)
	} else {
		f, err := os.Create(*output)
		if err != nil {
			log.Fatal(err)
		}

		_, err = f.WriteString(source)
		if err != nil {
			log.Fatal(err)
		}

		err = f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}
