package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/hasura/ndc-rest/ndc-rest-schema/utils"
	"gopkg.in/yaml.v3"
)

// Json2YamlCommandArguments represent available command arguments for the json2yaml command
type Json2YamlCommandArguments struct {
	File   string `help:"File path needs to be converted. Print to stdout if not set" required:"" short:"f"`
	Output string `help:"The location where the ndc schema file will be generated"    short:"o"`
}

// Json2Yaml converts a JSON file to YAML
func Json2Yaml(args *Json2YamlCommandArguments, logger *slog.Logger) error {
	rawContent, err := utils.ReadFileFromPath(args.File)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	var jsonContent any
	if err := json.Unmarshal(rawContent, &jsonContent); err != nil {
		slog.Error(err.Error())
		return err
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(jsonContent); err != nil {
		slog.Error(err.Error())
		return err
	}

	if args.Output != "" {
		if err := os.WriteFile(args.Output, buf.Bytes(), 0664); err != nil {
			slog.Error(err.Error())
			return err
		}
		logger.Info("generated successfully to " + args.Output)
		return nil
	}

	fmt.Print(buf.String())
	return nil
}
