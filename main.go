// This file is part of evmbind.

// Copyright (C) 2022 Ade M Ramdani.
// SPDX-License-Identifier: GPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.
//
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm/runtime"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:   "evmbind",
		Usage:  "generate Go bindings for EVM contracts",
		Action: binder,
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:     "abi",
				Usage:    "path to the ABI JSON file to bind against",
				Required: true,
			},
			&cli.PathFlag{
				Name:     "bin",
				Usage:    "path to the bytecode binary to bind against",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "pkg",
				Usage:    "name of the package to generate the bindings into",
				Required: true,
			},
			&cli.PathFlag{
				Name:     "out",
				Usage:    "path to the output dir",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "cr",
				Usage: "remove creation code from the binary",
			},
		},
	}

	app.Run(os.Args)
}

func removeCreationCode(bin string) string {
	code := common.Hex2Bytes(bin)
	ret, _, err := runtime.Execute(code, []byte{}, nil)
	if err != nil {
		panic(err)
	}

	return strings.TrimPrefix(hexutil.Encode(ret), "0x")
}

func binder(ctx *cli.Context) error {
	abiPath := ctx.Path("abi")
	src0, err := ioutil.ReadFile(abiPath)
	if err != nil {
		return err
	}

	binPath := ctx.Path("bin")
	src1, err := ioutil.ReadFile(binPath)
	if err != nil {
		return err
	}

	// stringify abi
	var abiRaw json.RawMessage
	err = json.Unmarshal(src0, &abiRaw)
	if err != nil {
		return err
	}

	abiStr, err := json.Marshal(abiRaw)
	if err != nil {
		return err
	}

	abivet := strings.ReplaceAll(string(abiStr), "\"", "\\\"")
	binvet := string(src1)

	if ctx.Bool("cr") {
		binvet = removeCreationCode(binvet)
	}

	var templateData TemplateData
	templateData.Package = ctx.String("pkg")
	templateData.ABI = abivet
	templateData.Bin = binvet

	vec, err := abi.JSON(strings.NewReader(string(src0)))
	if err != nil {
		return err
	}

	for _, method := range vec.Methods {
		var fn Function
		// fn.Name first letter is upper case
		fn.Name = strings.ToUpper(string(method.Name[0])) + string(method.Name[1:])
		fn.Method = method.Name
		fn.Id = hexutil.Encode(method.ID)
		fn.Raw = method.String()

		for _, input := range method.Inputs {
			args := Argument{
				Name: input.Name,
				Type: input.Type,
			}

			fn.Inputs = append(fn.Inputs, args)
		}

		for _, output := range method.Outputs {
			fn.Outputs = append(fn.Outputs, output.Type)
		}

		templateData.Funcs = append(templateData.Funcs, fn)
	}

	fnMap := map[string]any{
		"parseIn":   parseIn,
		"parseOut":  parseOut,
		"parseBody": parseBody,
	}

	templ := template.Must(template.New("").Funcs(fnMap).Parse(Templ))

	var b bytes.Buffer
	err = templ.Execute(&b, templateData)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath.Join(ctx.Path("out"), "evm.go"), b.Bytes(), 0644)
}

func bindType(kind abi.Type) string {
	switch kind.T {
	case abi.AddressTy:
		return "common.Address"
	case abi.IntTy, abi.UintTy:
		parts := regexp.MustCompile(`(u)?int([0-9]*)`).FindStringSubmatch(kind.String())
		switch parts[2] {
		case "8", "16", "32", "64":
			return fmt.Sprintf("%sint%s", parts[1], parts[2])
		}
		return "*big.Int"
	case abi.FixedBytesTy:
		return fmt.Sprintf("[%d]byte", kind.Size)
	case abi.BytesTy:
		return "[]byte"
	default:
		return kind.String()
	}
}

func parseIn(in []Argument) string {
	var s string
	for i, v := range in {
		if i > 0 {
			s += ", "
		}

		s += fmt.Sprintf("%s %s", v.Name, bindType(v.Type))
	}

	return s
}

func parseOut(out []abi.Type) string {
	var s string
	if len(out) > 1 {
		s += "("
	}

	for i, v := range out {
		if i > 0 {
			s += ", "
		}

		s += bindType(v)
	}

	if len(out) > 1 {
		s += ")"
	}

	return s
}

func parseBody(method string, input []Argument, output []abi.Type) string {
	var data tmpFnBodyData
	data.Method = method

	for _, v := range input {
		data.AbiPackParam += fmt.Sprintf(", %s", v.Name)
	}

	for i, v := range output {
		if i > 0 {
			data.Return += ", "
		}
		data.Return += fmt.Sprintf("res[%d].(%s)", i, bindType(v))
	}

	tmp, err := template.New("").Parse(tmpFnBody)
	if err != nil {
		panic(err)
	}

	var s strings.Builder
	err = tmp.Execute(&s, data)
	if err != nil {
		panic(err)
	}

	return s.String()
}

