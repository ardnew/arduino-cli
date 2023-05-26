// This file is part of arduino-cli.
//
// Copyright 2020 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package ctags

import (
	"strconv"
	"strings"

	"github.com/arduino/go-paths-helper"

	"github.com/arduino/arduino-cli/legacy/builder/types"
)

const KIND_PROTOTYPE = "prototype"
const KIND_FUNCTION = "function"

//const KIND_PROTOTYPE_MODIFIERS = "prototype_modifiers"

const TEMPLATE = "template"
const STATIC = "static"
const EXTERN = "extern \"C\""

var KNOWN_TAG_KINDS = map[string]bool{
	"prototype": true,
	"function":  true,
}

type CTagsParser struct {
	tags     []*types.CTag
	mainFile *paths.Path
}

func (p *CTagsParser) Parse(ctagsOutput []byte, mainFile *paths.Path) []*types.CTag {
	rows := strings.Split(string(ctagsOutput), "\n")
	rows = removeEmpty(rows)

	p.mainFile = mainFile

	for _, row := range rows {
		p.tags = append(p.tags, parseTag(row))
	}

	p.skipTagsWhere(tagIsUnknown)
	p.skipTagsWhere(tagIsUnhandled)
	p.addPrototypes()
	p.removeDefinedProtypes()
	p.skipDuplicates()
	p.skipTagsWhere(p.prototypeAndCodeDontMatch)

	return p.tags
}

func (p *CTagsParser) addPrototypes() {
	for _, tag := range p.tags {
		if !tag.SkipMe {
			addPrototype(tag)
		}
	}
}

func addPrototype(tag *types.CTag) {
	if strings.Index(tag.Prototype, TEMPLATE) == 0 {
		if strings.Index(tag.Code, TEMPLATE) == 0 {
			code := tag.Code
			if strings.Contains(code, "{") {
				code = code[:strings.Index(code, "{")]
			} else {
				code = code[:strings.LastIndex(code, ")")+1]
			}
			tag.Prototype = code + ";"
		} else {
			//tag.Code is 99% multiline, recreate it
			code := findTemplateMultiline(tag)
			tag.Prototype = code + ";"
		}
		return
	}

	tag.PrototypeModifiers = ""
	if strings.Contains(tag.Code, STATIC+" ") {
		tag.PrototypeModifiers = tag.PrototypeModifiers + " " + STATIC
	}

	// Extern "C" modifier is now added in FixCLinkageTagsDeclarations

	tag.PrototypeModifiers = strings.TrimSpace(tag.PrototypeModifiers)
}

func (p *CTagsParser) removeDefinedProtypes() {
	definedPrototypes := make(map[string]bool)
	for _, tag := range p.tags {
		if tag.Kind == KIND_PROTOTYPE {
			definedPrototypes[tag.Prototype] = true
		}
	}

	for _, tag := range p.tags {
		if definedPrototypes[tag.Prototype] {
			//if ctx.DebugLevel >= 10 {
			//	ctx.GetLogger().Fprintln(os.Stdout, constants.LOG_LEVEL_DEBUG, constants.MSG_SKIPPING_TAG_ALREADY_DEFINED, tag.FunctionName)
			//}
			tag.SkipMe = true
		}
	}
}

func (p *CTagsParser) skipDuplicates() {
	definedPrototypes := make(map[string]bool)

	for _, tag := range p.tags {
		if !definedPrototypes[tag.Prototype] && !tag.SkipMe {
			definedPrototypes[tag.Prototype] = true
		} else {
			tag.SkipMe = true
		}
	}
}

type skipFuncType func(tag *types.CTag) bool

func (p *CTagsParser) skipTagsWhere(skipFunc skipFuncType) {
	for _, tag := range p.tags {
		if !tag.SkipMe {
			skip := skipFunc(tag)
			//if skip && p.debugLevel >= 10 {
			//	ctx.GetLogger().Fprintln(os.Stdout, constants.LOG_LEVEL_DEBUG, constants.MSG_SKIPPING_TAG_WITH_REASON, tag.FunctionName, runtime.FuncForPC(reflect.ValueOf(skipFunc).Pointer()).Name())
			//}
			tag.SkipMe = skip
		}
	}
}

func removeTralingSemicolon(s string) string {
	return s[0 : len(s)-1]
}

func removeSpacesAndTabs(s string) string {
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, "\t", "", -1)
	return s
}

func tagIsUnhandled(tag *types.CTag) bool {
	return !isHandled(tag)
}

func isHandled(tag *types.CTag) bool {
	if tag.Class != "" {
		return false
	}
	if tag.Struct != "" {
		return false
	}
	if tag.Namespace != "" {
		return false
	}
	return true
}

func tagIsUnknown(tag *types.CTag) bool {
	return !KNOWN_TAG_KINDS[tag.Kind]
}

func parseTag(row string) *types.CTag {
	tag := &types.CTag{}
	parts := strings.Split(row, "\t")

	tag.FunctionName = parts[0]
	// This unescapes any backslashes in the filename. These
	// filenames that ctags outputs originate from the line markers
	// in the source, as generated by gcc. gcc escapes both
	// backslashes and double quotes, but ctags ignores any escaping
	// and just cuts off the filename at the first double quote it
	// sees. This means any backslashes are still escaped, and need
	// to be unescape, and any quotes will just break the build.
	tag.Filename = strings.Replace(parts[1], "\\\\", "\\", -1)

	parts = parts[2:]

	returntype := ""
	for _, part := range parts {
		if strings.Contains(part, ":") {
			colon := strings.Index(part, ":")
			field := part[:colon]
			value := strings.TrimSpace(part[colon+1:])
			switch field {
			case "kind":
				tag.Kind = value
			case "line":
				val, _ := strconv.Atoi(value)
				// TODO: Check err from strconv.Atoi
				tag.Line = val
			case "typeref":
				tag.Typeref = value
			case "signature":
				tag.Signature = value
			case "returntype":
				returntype = value
			case "class":
				tag.Class = value
			case "struct":
				tag.Struct = value
			case "namespace":
				tag.Namespace = value
			}
		}
	}
	tag.Prototype = returntype + " " + tag.FunctionName + tag.Signature + ";"

	if strings.Contains(row, "/^") && strings.Contains(row, "$/;") {
		tag.Code = row[strings.Index(row, "/^")+2 : strings.Index(row, "$/;")]
	}

	return tag
}

func removeEmpty(rows []string) []string {
	var newRows []string
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if len(row) > 0 {
			newRows = append(newRows, row)
		}
	}

	return newRows
}
