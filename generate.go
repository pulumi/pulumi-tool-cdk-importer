// Copyright 2016-2024, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build generate
// +build generate

//go:generate go run generate.go

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	awsNativeRepo   = "pulumi/pulumi-aws-native"
	awsNativeBranch = "t0yv0/import-ids"
)

func main() {
	remote := fetchRemote()
	fmt.Println(remote)

	src := filepath.Join(remote, "provider", "cmd", "pulumi-resource-aws-native", "metadata.json")
	copyFile(src, "pulumi-aws-native-metadata.json")
}

func copyFile(src, dest string) {
	srcFile, err := os.Open(src)
	if err != nil {
		log.Fatal(err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		log.Fatal(err)
	}
	defer destFile.Close()

	if _, err := srcFile.WriteTo(destFile); err != nil {
		log.Fatal(err)
	}
}

func fetchRemote() string {
	tmp := os.TempDir()
	dir := filepath.Join(tmp, strings.ReplaceAll(fmt.Sprintf("%s-%s", awsNativeRepo, awsNativeBranch), "/", "-"))
	stat, err := os.Stat(dir)
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
	if os.IsNotExist(err) || !stat.IsDir() {
		if err := os.Mkdir(dir, os.ModePerm); err != nil {
			log.Fatal(err)
		}
		repoUrl := "https://github.com/" + awsNativeRepo
		cmd := exec.Command("git", "clone", "--depth=1", "-b", awsNativeBranch, repoUrl, dir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
	}
	return dir
}
