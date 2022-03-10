package main

import (
	"github.com/bytesparadise/libasciidoc/pkg/plugins"
	"github.com/bytesparadise/libasciidoc/pkg/types"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
)

// map of diagram name to format string used to run program
var config = map[string]string{
	"plantuml": "java -jar /usr/share/plantuml/plantuml.jar -t%s -pipe > %s",
}

func isDiagram(element interface{}) bool {
	// diagrams are DelimitedBlocks
	block, ok := element.(*types.DelimitedBlock)
	if !ok {
		return false
	}
	// they have a first positional element
	diag, _, err := block.GetAttributes().GetAsString("@positional-1")
	if err != nil {
		return false
	}
	// that element must match one of our supported diagram configs
	_, ok = config[diag]
	if !ok {
		return false
	}
	// the block should have at least one element
	elements := block.GetElements()
	if len(elements) == 0 {
		return false
	}
	// and the first one should be a StringElement
	_, ok = elements[0].(*types.StringElement)
	if !ok {
		return false
	}
	return true
}

// converts a DelmitedBlock to an ImageBlock by calling a diagram program
// return original element and error message on failure
func makeDiagram(element interface{}) (interface{}, error) {
	// gather info from the element and run some checks
	block, ok := element.(*types.DelimitedBlock)
	if !ok {
		return element, errors.New("element type not DelimitedBlock")
	}
	diag, _, err := block.GetAttributes().GetAsString("@positional-1")
	if err != nil {
		return element, err
	}
	elements := block.GetElements()
	if len(elements) == 0 {
		return element, errors.New("DelimitedBlock does not contain enough elements")
	}
	stringElement, ok := elements[0].(*types.StringElement)
	if !ok {
		return element, errors.New("DelimitedBlock does not contain StringElement")
	}

	// we use an md5sum of the content if a target isn't provided
	content := stringElement.Content
	md5sum := md5.Sum([]byte(content))
	hash := hex.EncodeToString(md5sum[:])
	format := block.GetAttributes().GetAsStringWithDefault("format", "svg")
	default_target := hash + "." + format
	target := block.GetAttributes().GetAsStringWithDefault("target", default_target)
	log.Debugf("Converting diagram: %s target: %s format: %s content:\n%s\n", diag, target, format, content)

	// run the diagram cmd and feed it content through a pipe
	cmd := fmt.Sprintf(config[diag], format, target)
	log.Debugf("Running command: %s\n", cmd)
	subProcess := exec.Command("bash", "-c", cmd)
	stdin, err := subProcess.StdinPipe()
	if err != nil {
		return element, err
	}
	err = subProcess.Start()
	if err != nil {
		return element, err
	}
	io.WriteString(stdin, content)
	stdin.Close()
	subProcess.Wait()

	// create an ImageBlock and return it
	path := []interface{}{target}
	location, err := types.NewLocation("", path)
	if err != nil {
		return element, err
	}
	newBlock, err := types.NewImageBlock(location, block.GetAttributes())
	if err != nil {
		return element, err
	}
	return newBlock, nil
}

// used to go through all of the elements in the AST
func walkElements(element interface{}) interface{} {
	if isDiagram(element) {
		newBlock, err := makeDiagram(element)
		if err != nil {
			log.Error(err)
		}
		return newBlock
	} else {
		switch elem := element.(type) {
		case types.WithElements:
			var newElements []interface{}
			for _, el := range elem.GetElements() {
				newElements = append(newElements, walkElements(el))
			}
			elem.SetElements(newElements)
			return elem
		}
		return element
	}
}

var PreRender plugins.PreRenderFunc = func(doc *types.Document) (*types.Document, error) {
	// walkElements already does this
	// we could simplify if Document supported WithElements
	var newElements []interface{}
	for _, element := range doc.Elements {
		newElements = append(newElements, walkElements(element))
	}
	return doc, nil
}
