package goxsd9_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/goxdra/goxsd9"
)

func ExampleValidateInstance() {
	root, err := goxsd9.NewResolvedSource(
		context.Background(),
		"schema.xsd",
		io.NopCloser(strings.NewReader(`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:example"><xs:element name="count" type="xs:integer"/></xs:schema>`)),
	)
	if err != nil {
		fmt.Println("NewResolvedSource:", err)
		return
	}
	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		fmt.Println("ParseSchema:", err)
		return
	}
	err = goxsd9.ValidateInstance(
		schema,
		"instance.xml",
		io.NopCloser(strings.NewReader(`<count xmlns="urn:example">42</count>`)),
	)
	fmt.Println(err)
	// Output:
	// <nil>
}
