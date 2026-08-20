package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDecodeInstanceBuildsOrderedLocatedTree(t *testing.T) { //nolint:gocognit // explicit assertions document ordered representation facts.
	input := `<root xmlns="urn:root" xmlns:p="urn:p" a="one" p:a="two">A<!--comment--><?note data?>B<p:child p:b="v"/>tail</root>`
	reader := &instanceTrackingSource{data: []byte(input)}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "instance.xml"})
	if err != nil {
		t.Fatalf("decodeInstance: %v", err)
	}
	if document == nil {
		t.Fatal("decodeInstance returned a nil document")
	}
	if got, want := document.source, SourceID("instance.xml"); got != want {
		t.Fatalf("document source = %q, want %q", got, want)
	}
	if got, want := document.root.name, (syntaxName{namespace: "urn:root", local: "root"}); got != want {
		t.Fatalf("root name = %#v, want %#v", got, want)
	}
	assertLoc(t, document.root.loc, 1, 1)
	if got, want := len(document.root.attrs), 2; got != want {
		t.Fatalf("root attribute count = %d, want %d", got, want)
	}
	if got, want := document.root.attrs[0].name, (syntaxName{local: "a"}); got != want {
		t.Fatalf("first attribute name = %#v, want %#v", got, want)
	}
	if got, want := document.root.attrs[1].name, (syntaxName{namespace: "urn:p", local: "a"}); got != want {
		t.Fatalf("second attribute name = %#v, want %#v", got, want)
	}
	if got, want := document.root.attrs[0].value, "one"; got != want {
		t.Fatalf("first attribute value = %q, want %q", got, want)
	}
	if namespace, ok := document.root.scope.lookup(""); !ok || namespace != "urn:root" {
		t.Fatalf("default namespace = %q, %t; want urn:root, true", namespace, ok)
	}
	if namespace, ok := document.root.scope.lookup("p"); !ok || namespace != "urn:p" {
		t.Fatalf("p namespace = %q, %t; want urn:p, true", namespace, ok)
	}

	if got, want := len(document.root.children), 4; got != want {
		t.Fatalf("root child count = %d, want %d", got, want)
	}
	assertInstanceText(t, document.root.children[0], "A")
	assertInstanceText(t, document.root.children[1], "B")
	child, ok := document.root.children[2].(*instanceElement)
	if !ok {
		t.Fatalf("root child 2 = %T, want *instanceElement", document.root.children[2])
	}
	if got, want := child.name, (syntaxName{namespace: "urn:p", local: "child"}); got != want {
		t.Fatalf("child name = %#v, want %#v", got, want)
	}
	if got, want := len(child.attrs), 1; got != want {
		t.Fatalf("child attribute count = %d, want %d", got, want)
	}
	assertInstanceText(t, document.root.children[3], "tail")
	if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
		t.Fatalf("stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
	}
}

func TestDecodeInstanceAppliesNamespaceContextsAndAttributeWhitespace(t *testing.T) {
	input := `<p:root xmlns:p="relative" xmlns="urn:default" xml:lang="en" a="one` + "\t" + `two` + "\r\n" + `three" ref="&#xD;&#xA;&#x9;"><child xmlns=""><p:item/></child></p:root>`
	document, reader := decodeInstanceTestInput(t, input)
	if got, want := document.root.name, (syntaxName{namespace: "relative", local: "root"}); got != want {
		t.Fatalf("root name = %#v, want %#v", got, want)
	}
	if got, want := len(document.root.attrs), 3; got != want {
		t.Fatalf("root attribute count = %d, want %d", got, want)
	}
	if got, want := document.root.attrs[0].name, (syntaxName{namespace: xmlNamespaceURI, local: "lang"}); got != want {
		t.Fatalf("xml:lang name = %#v, want %#v", got, want)
	}
	if got, want := document.root.attrs[1].value, "one two three"; got != want {
		t.Fatalf("normalized attribute value = %q, want %q", got, want)
	}
	if got, want := document.root.attrs[2].value, "\r\n\t"; got != want {
		t.Fatalf("character-reference attribute value = %q, want %q", got, want)
	}
	child, ok := document.root.children[0].(*instanceElement)
	if !ok {
		t.Fatalf("root child = %T, want *instanceElement", document.root.children[0])
	}
	if got, want := child.name, (syntaxName{local: "child"}); got != want {
		t.Fatalf("child name = %#v, want %#v", got, want)
	}
	item, ok := child.children[0].(*instanceElement)
	if !ok {
		t.Fatalf("child child = %T, want *instanceElement", child.children[0])
	}
	if got, want := item.name, (syntaxName{namespace: "relative", local: "item"}); got != want {
		t.Fatalf("item name = %#v, want %#v", got, want)
	}
	if namespace, ok := child.scope.lookup(""); !ok || namespace != "" {
		t.Fatalf("child default namespace = %q, %t; want empty, true", namespace, ok)
	}
	if !reader.closed {
		t.Fatal("decoder did not close namespace test source")
	}
}

func TestDecodeInstanceRecordsAttributeLocations(t *testing.T) {
	document, _ := decodeInstanceTestInput(t, `<root a="1" b="2"/>`)
	if len(document.root.attrs) != 2 {
		t.Fatalf("root attribute count = %d, want 2", len(document.root.attrs))
	}
	assertLoc(t, document.root.attrs[0].loc, 1, 7)
	assertLoc(t, document.root.attrs[1].loc, 1, 13)

	document, _ = decodeInstanceTestInput(t, "<root\r\n  α=\"1\"\r\n  b=\"2\"/>")
	if len(document.root.attrs) != 2 {
		t.Fatalf("multiline root attribute count = %d, want 2", len(document.root.attrs))
	}
	assertLoc(t, document.root.attrs[0].loc, 2, 3)
	assertLoc(t, document.root.attrs[1].loc, 3, 3)
}

func TestDecodeInstanceAcceptsXMLDeclarationAndAttributeReferences(t *testing.T) {
	input := `<?xml version = "1.0" encoding = 'UTF-8' standalone = "yes"?><root refs="&lt;&gt;&amp;&apos;&quot;&#65;&#x42;"/>`
	document, reader := decodeInstanceTestInput(t, input)
	if len(document.root.attrs) != 1 {
		t.Fatalf("root attribute count = %d, want 1", len(document.root.attrs))
	}
	if got, want := document.root.attrs[0].value, "<>&'\"AB"; got != want {
		t.Fatalf("reference-expanded attribute = %q, want %q", got, want)
	}
	if !reader.closed {
		t.Fatal("decoder did not close XML declaration source")
	}
}

func TestDecodeInstanceAcceptsStandaloneXMLDeclaration(t *testing.T) {
	decodeInstanceTestInput(t, `<?xml version="1.0" standalone="yes"?><root/>`)
}

func TestDecodeInstanceRejectsDuplicateOrMisorderedXMLDeclarationFields(t *testing.T) {
	for _, input := range []string{
		`<?xml version="1.0" standalone="yes" standalone="no"?><root/>`,
		`<?xml version="1.0" standalone="yes" encoding="UTF-8"?><root/>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("invalid XML declaration returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), InvalidInstanceXMLCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceDefersWhitespaceSeparatedNonUTF8Encoding(t *testing.T) {
	for _, input := range []string{
		`<?xml version = "1.0" encoding = "UTF-16"?><root/>`,
		"<?xml version\t=\t\"1.0\" encoding\t=\t\"UTF-16\"?><root/>",
		"<?xml version\n=\n\"1.0\" encoding\n=\n\"UTF-16\"?><root/>",
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("unsupported encoding returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureUnsupported; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			if got, want := diagnostic.SpecRef(), instanceXMLEncodingSpecRef; got != want {
				t.Fatalf("SpecRef() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceNormalizesBOMLineEndingsAndCountsUnicodeColumns(t *testing.T) {
	for _, separator := range []string{"\r", "\r\n"} {
		t.Run(strings.ReplaceAll(separator, "\r", "CR"), func(t *testing.T) {
			input := "\ufeff<root>" + separator + "😀<child a=\"v\"/></root>"
			document, _ := decodeInstanceTestInput(t, input)
			assertLoc(t, document.root.loc, 1, 1)
			child, ok := document.root.children[1].(*instanceElement)
			if !ok {
				t.Fatalf("root child 1 = %T, want *instanceElement", document.root.children[1])
			}
			assertLoc(t, child.loc, 2, 2)
		})
	}

	document, _ := decodeInstanceTestInput(t, "<root>literal&#xD;&#xA;value</root>")
	text, ok := document.root.children[0].(instanceText)
	if !ok {
		t.Fatalf("root text = %T, want instanceText", document.root.children[0])
	}
	if got, want := text.data, "literal\r\nvalue"; got != want {
		t.Fatalf("character data = %q, want %q", got, want)
	}
}

func TestDecodeInstanceRejectsInvalidNamespacesAndDuplicateExpandedAttributes(t *testing.T) { //nolint:gocognit // table cases assert each diagnostic contract explicitly.
	tests := []struct {
		name  string
		input string
		code  string
		ref   string
	}{
		{name: "unbound prefix", input: `<p:root/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "prefix undeclaration", input: `<root xmlns:p="urn:p"><p:item xmlns:p=""/></root>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "xml URI default", input: `<root xmlns="` + xmlNamespaceURI + `"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "xml URI alias", input: `<root xmlns:p="` + xmlNamespaceURI + `"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "bad xml binding", input: `<root xmlns:xml="urn:not-xml"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "xmlns prefix", input: `<root xmlns:xmlns="urn:not-xmlns"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "invalid prefixed local name", input: `<p:9 xmlns:p="urn:p"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "empty namespace prefix", input: `<:root/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "invalid namespace declaration prefix", input: `<root xmlns:9="urn:p"/>`, code: InvalidInstanceNamespaceCode, ref: instanceXMLNamespacesSpecRef},
		{name: "duplicate expanded attributes", input: `<root xmlns:p="urn:x" xmlns:q="urn:x" p:a="1" q:a="2"/>`, code: InvalidInstanceXMLCode, ref: instanceXMLStartTagsSpecRef},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &instanceTrackingSource{data: []byte(test.input)}
			document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "invalid.xml"})
			if document != nil {
				t.Fatal("invalid instance returned a partial document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), test.code; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			if got, want := diagnostic.SpecRef(), test.ref; got != want {
				t.Fatalf("SpecRef() = %q, want %q", got, want)
			}
			if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
				t.Fatalf("invalid stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
			}
		})
	}
}

func TestDecodeInstanceRejectsMalformedPrologRootsCommentsAndPIs(t *testing.T) { //nolint:gocognit // table cases assert each diagnostic contract explicitly.
	valid := " \n<!--before--><?note data?><?p:target?><root/> \n<!--after--><?tail?>"
	if document, reader := decodeInstanceTestInput(t, valid); document == nil || !reader.closed {
		t.Fatalf("valid surrounding markup rejected: document=%#v reader=%#v", document, reader)
	}
	tests := []struct {
		name  string
		input string
		class FailureClass
		code  string
	}{
		{name: "no root", input: " \n<!--comment-->", class: FailureInvalid, code: InvalidInstanceRootCode},
		{name: "multiple roots", input: "<one/><two/>", class: FailureInvalid, code: InvalidInstanceRootCode},
		{name: "text before root", input: "text<root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "text after root", input: "<root/>text", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "character reference before root", input: "&#x20;<root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "character reference after root", input: "<root/>&#x20;", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "CDATA before root", input: "<![CDATA[ ]]><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "CDATA after root", input: "<root/><![CDATA[ ]]>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "declaration after whitespace", input: " <?xml version=\"1.0\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "invalid declaration", input: "<?xml standalone=\"yes\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "invalid declaration value", input: "<?xml version=\"1.0\" standalone=\"maybe\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "invalid declaration field", input: "<?xml version=\"1.0\" extra=\"value\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "bad comment", input: "<root><!--bad--x--></root>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "unknown directive", input: "<!bogus><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "unclosed element", input: "<root><child/></root", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "DTD unsupported", input: "<!DOCTYPE root><root/>", class: FailureUnsupported, code: UnsupportedInstanceSyntaxCode},
		{name: "DTD after root", input: "<root/><!DOCTYPE root>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "DTD inside root", input: "<root><!DOCTYPE root></root>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "non-UTF-8 encoding without version", input: "<?xml encoding=\"UTF-16\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "non-UTF-8 encoding after root", input: "<root/><?xml version=\"1.0\" encoding=\"UTF-16\"?>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "malformed non-UTF-8 encoding", input: "<?xml version=\"1.0\" encoding=\"UTF 16\"?><root/>", class: FailureInvalid, code: InvalidInstanceXMLCode},
		{name: "non-UTF-8 encoding unsupported", input: "<?xml version=\"1.0\" encoding=\"UTF-16\"?><root/>", class: FailureUnsupported, code: UnsupportedInstanceSyntaxCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, test.input)
			if document != nil {
				t.Fatal("invalid or unsupported instance returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), test.class; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), test.code; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

func TestDecodeInstanceDefersWellFormedDTDUnsupported(t *testing.T) { //nolint:gocognit // table cases assert each diagnostic contract explicitly.
	tests := []struct {
		name  string
		input string
		loc   Loc
	}{
		{name: "simple", input: "<!DOCTYPE root><root/>", loc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "internal subset", input: "\n  <!DOCTYPE root [<!ELEMENT root EMPTY>]><root/>", loc: Loc{source: "instance.xml", line: 2, column: 3}},
		{name: "system identifier", input: "<!DOCTYPE root SYSTEM \"root.dtd\"><root/>", loc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "public identifier", input: "<!DOCTYPE root PUBLIC \"-//Example//DTD Root 1.0//EN\" \"root.dtd\"><root/>", loc: Loc{source: "instance.xml", line: 1, column: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &instanceTrackingSource{data: []byte(test.input)}
			document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "instance.xml"})
			if document != nil {
				t.Fatal("unsupported DTD returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureUnsupported; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			if got, want := diagnostic.SpecRef(), instanceXMLDTDSpecRef; got != want {
				t.Fatalf("SpecRef() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Loc(), test.loc; got != want {
				t.Fatalf("Loc() = %v, want %v", got, want)
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic does not match ErrUnsupported: %v", err)
			}
			if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
				t.Fatalf("stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
			}
		})
	}
}

func TestDecodeInstanceAcceptsBoundedDTDSubsetItems(t *testing.T) {
	input := `<!DOCTYPE p:root [
<!-- a DTD comment -->
<?dtd instruction?>
%decl;
<!ELEMENT p:root (child)>
<!ELEMENT mixed (#PCDATA|child)*>
<!ELEMENT text (#PCDATA)>
<!ATTLIST p:root id CDATA #IMPLIED refs IDREFS #REQUIRED kind (one | two) "one" notation NOTATION (image) #IMPLIED>
<!ENTITY literal "[value]">
<!ENTITY external SYSTEM "external.ent">
<!ENTITY unparsed SYSTEM "image.bin" NDATA image>
<!ENTITY % parameter SYSTEM "parameter.ent">
<!NOTATION image SYSTEM "image/gif">
<!NOTATION text PUBLIC "-//Example//NOTATION Text 1.0//EN">
]><p:root xmlns:p="urn:p"/>`
	document, err := decodeInstanceFailureInput(t, input)
	if document != nil {
		t.Fatal("unsupported DTD returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.SpecRef(), instanceXMLDTDSpecRef; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceAcceptsDTDGrammarVariants(t *testing.T) {
	for _, input := range []string{
		`<!DOCTYPE root [<!ELEMENT root (child*)>]><root/>`,
		`<!DOCTYPE root [<!ELEMENT root (a?,b+,c*)>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root format NOTATION (gif | jpg) #IMPLIED>]><root/>`,
		`<!DOCTYPE root [<!ATTLIST root format NOTATION (gif|jpg) #IMPLIED>]><root/>`,
		`<!DOCTYPE root [<!ENTITY image SYSTEM "image.bin" NDATA image>]><root/>`,
		`<!DOCTYPE root [<!NOTATION image PUBLIC "pubid" "image.bin">]><root/>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("DTD grammar variant returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureUnsupported; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceAcceptsDTDProcessingInstructionData(t *testing.T) {
	for _, input := range []string{
		`<!DOCTYPE root [<?foo >?>]><root/>`,
		`<!DOCTYPE root [<?foo '?>]><root/>`,
		`<!DOCTYPE root [<?foo <<?>]><root/>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("DTD PI variant returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureUnsupported; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestInstanceLexicalReaderMasksDTDPIDataWithoutChangingLength(t *testing.T) {
	input := `<!DOCTYPE root [<?foo >?>]><root/>`
	positions := newSyntaxPositionReader("instance.xml", strings.NewReader(input))
	reader := &instanceLexicalReader{positions: positions}
	var transformed strings.Builder
	for {
		value, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("ReadByte: %v", err)
		}
		transformed.WriteByte(value)
	}
	if got, want := transformed.String(), `<!DOCTYPE root [<?foo  ?>]><root/>`; got != want {
		t.Fatalf("transformed DTD PI = %q, want %q", got, want)
	}
	if got, want := positions.offset, int64(len(input)); got != want {
		t.Fatalf("transformed byte length = %d, want %d", got, want)
	}
}

func TestInstanceLexicalReaderDoesNotMaskNonDTDDirectiveData(t *testing.T) {
	for _, input := range []string{
		`<![CDATA[<!DOCTYPE fake><?foo >?>]]>`,
		`<!--<!DOCTYPE fake><?foo >?>-->`,
		`<?pi <!DOCTYPE fake>?>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			positions := newSyntaxPositionReader("instance.xml", strings.NewReader(input))
			reader := &instanceLexicalReader{positions: positions}
			reader.beginCapture(0)
			var transformed strings.Builder
			for {
				value, err := reader.ReadByte()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("ReadByte: %v", err)
				}
				transformed.WriteByte(value)
			}
			if got := transformed.String(); got != input {
				t.Fatalf("non-DTD transform = %q, want %q", got, input)
			}
		})
	}
}

func TestDecodeInstancePreservesCDATAContainingDirectiveText(t *testing.T) {
	input := `<root><![CDATA[<!DOCTYPE fake><?foo >?>]]></root>`
	document, _ := decodeInstanceTestInput(t, input)
	text, ok := document.root.children[0].(instanceText)
	if !ok {
		t.Fatalf("root child = %T, want instanceText", document.root.children[0])
	}
	if got, want := text.data, `<!DOCTYPE fake><?foo >?>`; got != want {
		t.Fatalf("CDATA text = %q, want %q", got, want)
	}
}

func TestDecodeInstancePreservesNonDTDDirectiveText(t *testing.T) {
	input := `<root><!--<!DOCTYPE fake><?foo >?>--><?pi <!DOCTYPE fake>?></root>`
	document, _ := decodeInstanceTestInput(t, input)
	if len(document.root.children) != 0 {
		t.Fatalf("non-DTD directive children = %d, want 0", len(document.root.children))
	}
}

func TestDecodeInstanceDTDStructuralErrorsRemainInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  string
	}{
		{name: "unclosed root", input: "<!DOCTYPE root><root>", code: InvalidInstanceXMLCode},
		{name: "trailing text", input: "<!DOCTYPE root><root/>text", code: InvalidInstanceXMLCode},
		{name: "multiple roots", input: "<!DOCTYPE root><one/><two/>", code: InvalidInstanceRootCode},
		{name: "duplicate DTD", input: "<!DOCTYPE root><!DOCTYPE other><root/>", code: InvalidInstanceXMLCode},
		{name: "DTD after root", input: "<!DOCTYPE root><root/><!DOCTYPE root>", code: InvalidInstanceXMLCode},
		{name: "DTD then XML declaration", input: "<!DOCTYPE root><?xml version=\"1.0\"?><root/>", code: InvalidInstanceXMLCode},
		{name: "unterminated internal subset", input: "<!DOCTYPE root [<!ELEMENT root EMPTY>><root/>", code: InvalidInstanceXMLCode},
		{name: "unterminated system literal", input: "<!DOCTYPE root SYSTEM \"root.dtd><root/>", code: InvalidInstanceXMLCode},
		{name: "invalid internal subset suffix", input: "<!DOCTYPE root [] extra><root/>", code: InvalidInstanceXMLCode},
		{name: "malformed internal comment", input: "<!DOCTYPE root [<!--bad--x-->]><root/>", code: InvalidInstanceXMLCode},
		{name: "arbitrary internal text", input: "<!DOCTYPE root [garbage]><root/>", code: InvalidInstanceXMLCode},
		{name: "malformed parameter entity", input: "<!DOCTYPE root [%decl]><root/>", code: InvalidInstanceXMLCode},
		{name: "malformed declaration boundary", input: "<!DOCTYPE root [<!ELEMENT root EMPTY]]><root/>", code: InvalidInstanceXMLCode},
		{name: "conditional section", input: "<!DOCTYPE root [<![INCLUDE[<!ELEMENT root EMPTY>]]>]><root/>", code: InvalidInstanceXMLCode},
		{name: "nested internal bracket", input: "<!DOCTYPE root [[literal]]><root/>", code: InvalidInstanceXMLCode},
		{name: "invalid DTD QName prefix", input: "<!DOCTYPE :root><root/>", code: InvalidInstanceXMLCode},
		{name: "invalid DTD QName local", input: "<!DOCTYPE root:><root/>", code: InvalidInstanceXMLCode},
		{name: "multiple DTD QName separators", input: "<!DOCTYPE a:b:c><root/>", code: InvalidInstanceXMLCode},
		{name: "ELEMENT QName separators", input: "<!DOCTYPE root [<!ELEMENT a:b:c EMPTY>]><root/>", code: InvalidInstanceXMLCode},
		{name: "content QName separators", input: "<!DOCTYPE root [<!ELEMENT root (a:b:c)>]><root/>", code: InvalidInstanceXMLCode},
		{name: "ATTLIST element QName separators", input: "<!DOCTYPE root [<!ATTLIST a:b:c id CDATA #IMPLIED>]><root/>", code: InvalidInstanceXMLCode},
		{name: "ATTLIST attribute QName separators", input: "<!DOCTYPE root [<!ATTLIST root a:b:c CDATA #IMPLIED>]><root/>", code: InvalidInstanceXMLCode},
		{name: "invalid PUBLIC literal", input: "<!DOCTYPE root PUBLIC \"-//Example<DTD\" \"root.dtd\"><root/>", code: InvalidInstanceXMLCode},
		{name: "ELEMENT missing contentspec", input: "<!DOCTYPE root [<!ELEMENT root>]><root/>", code: InvalidInstanceXMLCode},
		{name: "mixed contentspec missing repetition", input: "<!DOCTYPE root [<!ELEMENT root (#PCDATA|child)>]><root/>", code: InvalidInstanceXMLCode},
		{name: "ATTLIST missing type", input: "<!DOCTYPE root [<!ATTLIST root id>]><root/>", code: InvalidInstanceXMLCode},
		{name: "ENTITY missing definition", input: "<!DOCTYPE root [<!ENTITY root>]><root/>", code: InvalidInstanceXMLCode},
		{name: "NOTATION missing identifier", input: "<!DOCTYPE root [<!NOTATION image>]><root/>", code: InvalidInstanceXMLCode},
		{name: "ENTITY NDATA missing space", input: "<!DOCTYPE root [<!ENTITY image SYSTEM \"image.bin\"NDATA image>]><root/>", code: InvalidInstanceXMLCode},
		{name: "NOTATION system literal missing space", input: "<!DOCTYPE root [<!NOTATION image PUBLIC \"pubid\"\"image.bin\">]><root/>", code: InvalidInstanceXMLCode},
		{name: "DTD PI question boundary", input: "<!DOCTYPE root [<?foo?x?>]><root/>", code: InvalidInstanceXMLCode},
		{name: "NUL in system literal", input: "<!DOCTYPE root SYSTEM \"\x00\"><root/>", code: InvalidInstanceXMLCode},
		{name: "NUL in DTD comment", input: "<!DOCTYPE root [<!--\x00-->]><root/>", code: InvalidInstanceXMLCode},
		{name: "NUL in DTD PI", input: "<!DOCTYPE root [<?foo \x00?>]><root/>", code: InvalidInstanceXMLCode},
		{name: "NUL in DTD declaration", input: "<!DOCTYPE root [<!ELEMENT root (child\x00)>]><root/>", code: InvalidInstanceXMLCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, test.input)
			if document != nil {
				t.Fatal("malformed DTD instance returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), test.code; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceRejectsAttributesWithoutSeparatingSpace(t *testing.T) {
	for _, input := range []string{
		`<root a="1"b="2"/>`,
		`<!DOCTYPE root><root a="1"b="2"/>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("malformed start tag returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), InvalidInstanceXMLCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceDefersDTDNamedEntityReferences(t *testing.T) {
	for _, input := range []string{
		`<!DOCTYPE root [<!ENTITY e "text">]><root>&e;</root>`,
		`<!DOCTYPE root SYSTEM "root.dtd"><root>&external;</root>`,
		`<!DOCTYPE root [<!ENTITY e "text">]><root a="&e;"/>`,
		`<!DOCTYPE root [<!ENTITY e "text">]><root><![CDATA[&not-a-reference]]></root>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("DTD-dependent entity returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureUnsupported; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceDTDBoundaryMatrix(t *testing.T) { //nolint:gocognit // table cases assert the DTD boundary contract.
	tests := []struct {
		name      string
		input     string
		wantClass FailureClass
		wantCode  string
		wantSpec  string
		wantLoc   Loc
		wantDoc   bool
	}{
		{name: "basic DTD", input: `<!DOCTYPE root><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "top-level PE alone", input: `<!DOCTYPE root [%p;]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "top-level PE adjacent declaration", input: `<!DOCTYPE root [%p;<!ELEMENT root EMPTY>]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "top-level PE garbage", input: `<!DOCTYPE root [%p; garbage]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "top-level malformed PE", input: `<!DOCTYPE root [%p]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "top-level bare percent", input: `<!DOCTYPE root [%]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "legal parameter entity declaration marker", input: `<!DOCTYPE root [<!ENTITY % p "x">]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "trailing top-level PE", input: `<!DOCTYPE root [<!ELEMENT root EMPTY>%p;]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "general entity value PE", input: `<!DOCTYPE root [<!ENTITY e "%p;">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "parameter entity value PE", input: `<!DOCTYPE root [<!ENTITY % p "%q;">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "general entity value bare percent", input: `<!DOCTYPE root [<!ENTITY e "%">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "parameter entity value malformed PE", input: `<!DOCTYPE root [<!ENTITY % p "%q">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "direct PE in ELEMENT", input: `<!DOCTYPE root [<!ELEMENT %p; EMPTY>]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "direct PE in ELEMENT content model", input: `<!DOCTYPE root [<!ELEMENT root (%p;)>]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "direct PE in ATTLIST", input: `<!DOCTYPE root [<!ATTLIST %p; id CDATA #IMPLIED>]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "direct PE in ATTLIST declaration", input: `<!DOCTYPE root [<!ATTLIST root %p;>]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "direct PE in NOTATION", input: `<!DOCTYPE root [<!NOTATION %p; SYSTEM "image/gif">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in system literal", input: `<!DOCTYPE root SYSTEM "%p;"><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in public literal", input: `<!DOCTYPE root PUBLIC "%p;" "root.dtd"><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in ATTLIST AttValue", input: `<!DOCTYPE root [<!ATTLIST root a CDATA "%p;">]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in comment", input: `<!DOCTYPE root [<!-- %p; -->]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in PI", input: `<!DOCTYPE root [<?p %p;?>]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in entity CharRef", input: `<!DOCTYPE root [<!ENTITY e "&#37;p;">]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in entity hexadecimal CharRef", input: `<!DOCTYPE root [<!ENTITY e "&#x25;p;">]><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "percent in document text", input: `<root>%p;</root>`, wantDoc: true},
		{name: "colon-bearing PE reference", input: `<!DOCTYPE root [%p:q;]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "colon-bearing PE declaration name", input: `<!DOCTYPE root [<!ENTITY % p:q "x">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "colon-bearing entity declaration name", input: `<!DOCTYPE root [<!ENTITY p:q "x">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "colon-bearing entity reference name", input: `<!DOCTYPE root [<!ENTITY e "x">]><root>&p:q;</root>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 40}},
		{name: "colon-bearing NDATA notation name", input: `<!DOCTYPE root [<!ENTITY e SYSTEM "image.bin" NDATA p:q>]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "colon-bearing notation declaration name", input: `<!DOCTYPE root [<!NOTATION p:q SYSTEM "image/gif">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "external DTD", input: `<!DOCTYPE root SYSTEM "root.dtd"><root/>`, wantClass: FailureUnsupported, wantCode: UnsupportedInstanceSyntaxCode, wantSpec: instanceXMLDTDSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
		{name: "internal PE with external ID", input: `<!DOCTYPE root SYSTEM "root.dtd" [<!ENTITY e "%p;">]><root/>`, wantClass: FailureInvalid, wantCode: InvalidInstanceXMLCode, wantSpec: instanceXMLWellFormedSpecRef, wantLoc: Loc{source: "instance.xml", line: 1, column: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &instanceTrackingSource{data: []byte(test.input)}
			document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "instance.xml"})
			if test.wantDoc {
				if err != nil || document == nil {
					t.Fatalf("valid instance = document %#v, err %v", document, err)
				}
				if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
					t.Fatalf("stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
				}
				return
			}
			if document != nil {
				t.Fatal("invalid or unsupported DTD returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), test.wantClass; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), test.wantCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			if got, want := diagnostic.SpecRef(), test.wantSpec; got != want {
				t.Fatalf("SpecRef() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Loc(), test.wantLoc; got != want {
				t.Fatalf("Loc() = %v, want %v", got, want)
			}
			if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
				t.Fatalf("stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
			}
		})
	}
}

func TestDecodeInstanceDTDNamedEntityStructuralErrorsRemainPrimary(t *testing.T) {
	for _, input := range []string{
		`<!DOCTYPE root [<!ENTITY e "text">]><root>&e;`,
		`<!DOCTYPE root [<!ENTITY e "text">]><root/>&e;`,
		`<!DOCTYPE root [<!ENTITY e "text">]><root>&e</root>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("malformed DTD-dependent entity returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), InvalidInstanceXMLCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceDefersDTDDefaultNamespaceBinding(t *testing.T) {
	for _, input := range []string{
		`<!DOCTYPE p:root [<!ATTLIST p:root xmlns:p CDATA "urn:p">]><p:root/>`,
		`<!DOCTYPE p:root [<!ELEMENT p:root EMPTY><!ATTLIST p:root xmlns:p CDATA "urn:p">]><p:root/>`,
	} {
		document, err := decodeInstanceFailureInput(t, input)
		if document != nil {
			t.Fatal("DTD default namespace returned a document")
		}
		diagnostic := requireDiagnostic(t, err)
		if got, want := diagnostic.Class(), FailureUnsupported; got != want {
			t.Fatalf("Class() = %q, want %q", got, want)
		}
		if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
			t.Fatalf("Code() = %q, want %q", got, want)
		}
	}

	input := `<!DOCTYPE p:root [<!ATTLIST p:root xmlns:p CDATA "urn:p">]><p:root>`
	document, err := decodeInstanceFailureInput(t, input)
	if document != nil {
		t.Fatal("malformed DTD default namespace returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("malformed document Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), InvalidInstanceXMLCode; got != want {
		t.Fatalf("malformed document Code() = %q, want %q", got, want)
	}

	document, err = decodeInstanceFailureInput(t, `<!DOCTYPE p:root [<!ATTLIST p:root xmlns:p CDATA #IMPLIED>]><p:root/>`)
	if document != nil {
		t.Fatal("DTD implied namespace returned a document")
	}
	diagnostic = requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("DTD implied namespace Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), InvalidInstanceNamespaceCode; got != want {
		t.Fatalf("DTD implied namespace Code() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceDefersUnresolvedDTDNamespaceForExternalDefaults(t *testing.T) {
	// An external subset may provide the missing namespace declaration, so the
	// instance remains structurally deferred rather than being rejected early.
	document, err := decodeInstanceFailureInput(t, `<!DOCTYPE root SYSTEM "root.dtd"><p:root/>`)
	if document != nil {
		t.Fatal("DTD namespace fallback returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}

	document, err = decodeInstanceFailureInput(t, `<!DOCTYPE root><p:root/>`)
	if document != nil {
		t.Fatal("simple DTD namespace fallback returned a document")
	}
	diagnostic = requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureInvalid; got != want {
		t.Fatalf("simple DTD Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), InvalidInstanceNamespaceCode; got != want {
		t.Fatalf("simple DTD Code() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceRequiresLexicalEndElementNameMatch(t *testing.T) {
	for _, input := range []string{
		`<p:root xmlns:p="urn:x" xmlns:q="urn:x"></q:root>`,
		`<root xmlns="urn:x" xmlns:p="urn:x"></p:root>`,
		`<!DOCTYPE p:root><p:root xmlns:p="urn:x"></q:root>`,
		`<!DOCTYPE root><root xmlns="urn:x" xmlns:p="urn:x"></p:root>`,
	} {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, input)
			if document != nil {
				t.Fatal("lexically mismatched end element returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), InvalidInstanceXMLCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
		})
	}
}

func TestDecodeInstanceNamespaceDiagnosticsUseDeclarationLocations(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		line   int
		column int
	}{
		{name: "empty URI", input: "<root\n  α=\"😀\" xmlns:p=\"\"\n/>", line: 2, column: 9},
		{name: "duplicate declaration", input: "<root\n  α=\"😀\" xmlns:p=\"urn:p\" xmlns:p=\"urn:q\"\n/>", line: 2, column: 25},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := decodeInstanceFailureInput(t, test.input)
			if document != nil {
				t.Fatal("invalid namespace binding returned a document")
			}
			diagnostic := requireDiagnostic(t, err)
			if got, want := diagnostic.Class(), FailureInvalid; got != want {
				t.Fatalf("Class() = %q, want %q", got, want)
			}
			if got, want := diagnostic.Code(), InvalidInstanceNamespaceCode; got != want {
				t.Fatalf("Code() = %q, want %q", got, want)
			}
			assertLoc(t, diagnostic.Loc(), test.line, test.column)
		})
	}
}

func TestDecodeInstanceAcceptsXMLNameExtenderU02D0(t *testing.T) {
	if !validNCName("a\u02d0") {
		t.Fatal("U+02D0 is not accepted by the XML 1.0 NCName helper")
	}
	document, reader := decodeInstanceTestInput(t, "<root a\u02d0=\"1\"/>")
	if document == nil || !reader.closed {
		t.Fatalf("U+02D0 namespace name decode = document %#v, reader %#v", document, reader)
	}
	document, err := decodeInstanceFailureInput(t, "<!DOCTYPE a\u02d0:root><root/>")
	if document != nil {
		t.Fatal("DTD with U+02D0 QName returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("DTD with U+02D0 QName Class() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceDTDRootNameMismatchRemainsUnsupported(t *testing.T) {
	document, err := decodeInstanceFailureInput(t, "<!DOCTYPE declared><actual/>")
	if document != nil {
		t.Fatal("DTD root-name mismatch returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceAcceptsPrefixedDTDNameWithoutMatching(t *testing.T) {
	document, err := decodeInstanceFailureInput(t, `<!DOCTYPE p:declared><p:actual xmlns:p="urn:p"/>`)
	if document != nil {
		t.Fatal("DTD root-name mismatch returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
}

func TestDecodeInstanceRejectsUTF16BOM(t *testing.T) {
	data := []byte{0xff, 0xfe, '<', 0, 'r', 0, 'o', 0, 'o', 0, 't', 0, '/', 0, '>', 0}
	reader := &instanceTrackingSource{data: data}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "utf16.xml"})
	if document != nil {
		t.Fatal("UTF-16 instance returned a document")
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Class(), FailureUnsupported; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), UnsupportedInstanceSyntaxCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.SpecRef(), instanceXMLEncodingSpecRef; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported diagnostic does not match ErrUnsupported: %v", err)
	}
	if !reader.closed || reader.offset != len(reader.data) || reader.closeCalls != 1 {
		t.Fatalf("UTF-16 stream lifecycle = closed %t, offset %d, close calls %d", reader.closed, reader.offset, reader.closeCalls)
	}
}

func TestDecodeInstancePreservesReadAndCloseCausesInOrder(t *testing.T) {
	readErr := errors.New("instance read failed")
	closeErr := errors.New("instance close failed")
	input := `<root><child></root>` + strings.Repeat("x", 32)
	reader := &instanceTrackingSource{
		data:     []byte(input),
		failAt:   len(`<root><child></root>`),
		readErr:  readErr,
		closeErr: closeErr,
	}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "failure.xml"})
	if document != nil || err == nil {
		t.Fatalf("failure decode = document %#v, err %v", document, err)
	}
	if !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("combined error lost read or close cause: %v", err)
	}
	diagnostics := syntaxDiagnostics(err)
	if got, want := len(diagnostics), 3; got != want {
		t.Fatalf("diagnostic count = %d, want %d: %#v", got, want, diagnostics)
	}
	if diagnostics[0].Code() != InvalidInstanceXMLCode || diagnostics[1].Code() != SourceReadCode || diagnostics[2].Code() != SourceCloseCode {
		t.Fatalf("diagnostic order = %#v, want XML, read, close", diagnostics)
	}
	if !reader.closed || reader.closeCalls != 1 || reader.offset != reader.failAt {
		t.Fatalf("failure stream lifecycle = closed %t, close calls %d, offset %d, want offset %d", reader.closed, reader.closeCalls, reader.offset, reader.failAt)
	}
}

func TestDecodeInstanceCloseFailureReturnsNoDocument(t *testing.T) {
	closeErr := errors.New("close failed")
	reader := &instanceTrackingSource{data: []byte(`<root/>`), closeErr: closeErr}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "close.xml"})
	if document != nil || err == nil {
		t.Fatalf("close failure decode = document %#v, err %v", document, err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("close failure cause is not observable: %v", err)
	}
	diagnostic := requireDiagnostic(t, err)
	if got, want := diagnostic.Code(), SourceCloseCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if reader.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", reader.closeCalls)
	}
}

func TestDecodeInstanceReplayFuzzCorpus(t *testing.T) {
	for _, input := range readInstanceFuzzCorpus(t) {
		first := fuzzDecodeInstanceRun(t, input)
		second := fuzzDecodeInstanceRun(t, input)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("fuzz corpus result is not deterministic for %q:\nfirst=%#v\nsecond=%#v", input, first, second)
		}
	}
}

func FuzzDecodeInstance(f *testing.F) {
	for _, input := range []string{
		`<root/>`,
		`<root xmlns="urn:r" xmlns:p="urn:p"><p:item a="1">text</p:item></root>`,
		"\ufeff \r\n<root>😀<!--c--><?pi?></root>",
		`<root xmlns:p="urn:x" p:a="1" q:a="2"/>`,
		`<root><child></root>`,
		`<!DOCTYPE root><root/>`,
		"not XML",
	} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		first := fuzzDecodeInstanceRun(t, input)
		second := fuzzDecodeInstanceRun(t, input)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("decoder result is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
		}
	})
}

type instanceFuzzResult struct {
	document     *instanceDocument
	diagnostics  []instanceFuzzDiagnostic
	closed       bool
	consumedByte int
}

type instanceFuzzDiagnostic struct {
	class     FailureClass
	code      string
	feature   FeatureID
	loc       Loc
	message   string
	specRef   string
	causeType string
	cause     string
}

func fuzzDecodeInstanceRun(t testing.TB, input string) instanceFuzzResult {
	t.Helper()
	reader := &instanceTrackingSource{data: []byte(input)}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "fuzz.xml"})
	if document != nil && err != nil {
		t.Fatalf("decoder returned a document with an error: %v", err)
	}
	if document == nil && err == nil {
		t.Fatal("decoder returned neither a document nor an error")
	}
	if !reader.closed || reader.closeCalls != 1 {
		t.Fatalf("decoder close state = closed %t, calls %d", reader.closed, reader.closeCalls)
	}
	if reader.offset != len(reader.data) {
		t.Fatalf("decoder did not drain fuzz source: consumed %d of %d bytes", reader.offset, len(reader.data))
	}
	result := instanceFuzzResult{
		document:     document,
		closed:       reader.closed,
		consumedByte: reader.offset,
	}
	if err == nil {
		return result
	}
	for _, diagnostic := range syntaxDiagnostics(err) {
		cause := diagnostic.Unwrap()
		causeType := ""
		causeText := ""
		if cause != nil {
			causeType = fmt.Sprintf("%T", cause)
			causeText = cause.Error()
		}
		result.diagnostics = append(result.diagnostics, instanceFuzzDiagnostic{
			class:     diagnostic.Class(),
			code:      diagnostic.Code(),
			feature:   diagnostic.Feature(),
			loc:       diagnostic.Loc(),
			message:   diagnostic.message,
			specRef:   diagnostic.SpecRef(),
			causeType: causeType,
			cause:     causeText,
		})
	}
	if len(result.diagnostics) == 0 {
		t.Fatalf("decoder returned an error without diagnostics: %v", err)
	}
	return result
}

func readInstanceFuzzCorpus(t *testing.T) []string {
	t.Helper()
	directory := filepath.Join("testdata", "fuzz", "FuzzDecodeInstance")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read instance fuzz corpus: %v", err)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	inputs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		data, err := os.ReadFile(path) // #nosec G304 -- path is a checked-in fuzz corpus entry.
		if err != nil {
			t.Fatalf("read instance fuzz seed %q: %v", path, err)
		}
		lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
		if len(lines) != 2 || strings.TrimSpace(lines[0]) != "go test fuzz v1" {
			t.Fatalf("invalid instance fuzz seed %q", path)
		}
		encoded := strings.TrimSpace(lines[1])
		if !strings.HasPrefix(encoded, "string(") || !strings.HasSuffix(encoded, ")") {
			t.Fatalf("invalid instance fuzz seed value %q", path)
		}
		value, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(encoded, "string("), ")"))
		if err != nil {
			t.Fatalf("decode instance fuzz seed %q: %v", path, err)
		}
		inputs = append(inputs, value)
	}
	return inputs
}

func decodeInstanceTestInput(t *testing.T, input string) (*instanceDocument, *instanceTrackingSource) {
	t.Helper()
	reader := &instanceTrackingSource{data: []byte(input)}
	document, err := decodeInstance(reader, instanceDecodeConfig{sourceID: "instance.xml"})
	if err != nil {
		t.Fatalf("decodeInstance: %v", err)
	}
	if document == nil {
		t.Fatal("decodeInstance returned a nil document")
	}
	return document, reader
}

func decodeInstanceFailureInput(t *testing.T, input string) (*instanceDocument, error) {
	t.Helper()
	reader := &instanceTrackingSource{data: []byte(input)}
	return decodeInstance(reader, instanceDecodeConfig{sourceID: "instance.xml"})
}

func assertInstanceText(t *testing.T, node instanceNode, want string) {
	t.Helper()
	text, ok := node.(instanceText)
	if !ok {
		t.Fatalf("node = %T, want instanceText", node)
	}
	if text.data != want {
		t.Fatalf("text = %q, want %q", text.data, want)
	}
}

type instanceTrackingSource struct {
	data       []byte
	offset     int
	failAt     int
	readErr    error
	closeErr   error
	closed     bool
	closeCalls int
}

func (source *instanceTrackingSource) Read(buffer []byte) (int, error) {
	if source.failAt > 0 && source.offset >= source.failAt {
		return 0, source.readErr
	}
	if source.offset >= len(source.data) {
		return 0, io.EOF
	}
	limit := len(source.data)
	if source.failAt > 0 && source.failAt < limit {
		limit = source.failAt
	}
	n := copy(buffer, source.data[source.offset:limit])
	source.offset += n
	return n, nil
}

func (source *instanceTrackingSource) Close() error {
	source.closed = true
	source.closeCalls++
	return source.closeErr
}
