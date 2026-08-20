package goxsd9_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/goxdra/goxsd9"
)

type exampleParseResolver struct{}

func (exampleParseResolver) Resolve(
	ctx context.Context,
	namespaceURN, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	if schemaLocation != "child.xsd" || namespaceURN != "urn:child" {
		return goxsd9.ResolvedSource{}, fmt.Errorf("unexpected schema reference %q (%q)", namespaceURN, schemaLocation)
	}
	return goxsd9.NewResolvedSource(ctx, "child.xsd", io.NopCloser(strings.NewReader(
		`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:child"><xs:element name="child"/></xs:schema>`,
	)))
}

func ExampleParseSchema() {
	root, err := goxsd9.NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(
		`<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="urn:root"><xs:import namespace="urn:child" schemaLocation="child.xsd"/><xs:element name="root"/></xs:schema>`,
	)))
	if err != nil {
		fmt.Println("NewResolvedSource:", err)
		return
	}
	schema, err := goxsd9.ParseSchema(root, exampleParseResolver{})
	if err != nil {
		fmt.Println("ParseSchema:", err)
		return
	}
	childName, err := goxsd9.NewQName("urn:child", "child")
	if err != nil {
		fmt.Println("NewQName:", err)
		return
	}
	found := schema.Find(childName)
	fmt.Println("child components:", len(found))
	if err := schema.Walk(func(component goxsd9.Component) error {
		fmt.Println(component.Document(), component.Name())
		return nil
	}); err != nil {
		fmt.Println("Walk:", err)
		return
	}
	// Output:
	// child components: 1
	// root.xsd {urn:root}root
	// child.xsd {urn:child}child
}
