/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var goos = "linux"
var goarch = "amd64"
var outputdir = "bin"
var Bazel bool
var Gazelle bool
var BuildTargets []string

const (
	apiserverTarget  = "apiserver"
	controllerTarget = "controller"
)

var createBuildExecutablesCmd = &cobra.Command{
	Use:   "executables",
	Short: "Builds the source into executables to run on the local machine",
	Long:  `Builds the source into executables to run on the local machine`,
	Example: `# Generate code and build the apiserver and controller
# binaries in the bin directory so they can be run locally.
apiserver-boot build executables

# Build binaries into the linux/ directory using the cross compiler for linux:amd64
apiserver-boot build executables --goos linux --goarch amd64 --output linux/

# Regenerate Bazel BUILD files, and then build with bazel
# Must first install bazel and gazelle !!!
apiserver-boot build executables --bazel --gazelle

# Run Bazel without generating BUILD files
apiserver-boot build executables --bazel
`,
	Run: RunBuildExecutables,
}

func AddBuildExecutables(cmd *cobra.Command) {
	cmd.AddCommand(createBuildExecutablesCmd)

	createBuildExecutablesCmd.Flags().StringVar(&vendorDir, "vendor-dir", "", "Location of directory containing vendor files.")
	createBuildExecutablesCmd.Flags().StringVar(&goos, "goos", "", "if specified, set this GOOS")
	createBuildExecutablesCmd.Flags().StringVar(&goarch, "goarch", "", "if specified, set this GOARCH")
	createBuildExecutablesCmd.Flags().StringVar(&outputdir, "output", "bin", "if set, write the binaries to this directory")
	createBuildExecutablesCmd.Flags().BoolVar(&Bazel, "bazel", false, "if true, use bazel to build.  May require updating build rules with gazelle.")
	createBuildExecutablesCmd.Flags().BoolVar(&Gazelle, "gazelle", false, "if true, run gazelle before running bazel.")
	createBuildExecutablesCmd.Flags().StringArrayVar(&BuildTargets, "targets", []string{apiserverTarget, controllerTarget}, "The target binaries to build")
}

func RunBuildExecutables(cmd *cobra.Command, args []string) {
	if err := cmd.Flags().Parse(args); err != nil {
		klog.Fatal(err)
	}
	if Bazel {
		BazelBuild(cmd, args)
	} else {
		GoBuild(cmd, args)
	}
}

func BazelBuild(cmd *cobra.Command, args []string) {
	initApis()

	if Gazelle {
		if _, err := os.Stat("go.mod"); err == nil { // go mod exists
			// bazel - gomod integration
			c := exec.Command("bazel",
				"run",
				"//:gazelle",
				"--",
				"update-repos",
				"--from_file=go.mod",
				"--to_macro=repos.bzl%go_repositories",
				"--build_file_generation=on",
				"--build_file_proto_mode=disable",
				"--prune",
			)
			klog.Infof("%s", strings.Join(c.Args, " "))
			c.Stderr = os.Stderr
			c.Stdout = os.Stdout
			err := c.Run()
			if err != nil {
				klog.Fatal(err)
			}
		}

		c := exec.Command("bazel", "run", "//:gazelle")
		klog.Infof("%s", strings.Join(c.Args, " "))

		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err := c.Run()
		if err != nil {
			klog.Fatal(err)
		}
	}

	targetDirs := make([]string, 0)
	if buildApiserver() {
		targetDirs = append(targetDirs, filepath.Join("cmd", "apiserver"))
	}
	if buildController() {
		targetDirs = append(targetDirs, filepath.Join("cmd", "manager"))
	}
	c := exec.Command("bazel", append([]string{"build"}, targetDirs...)...)
	klog.Infof("%s", strings.Join(c.Args, " "))
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	err := c.Run()
	if err != nil {
		klog.Fatal(err)
	}

	os.RemoveAll(filepath.Join("bin", "apiserver"))
	os.RemoveAll(filepath.Join("bin", "controller-manager"))

	if buildApiserver() {
		c := exec.Command("cp",
			filepath.Join("bazel-bin", "cmd", "apiserver", "apiserver_", "apiserver"),
			filepath.Join("bin", "apiserver"))
		klog.Infof("%s", strings.Join(c.Args, " "))
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err := c.Run()
		if err != nil {
			klog.Fatal(err)
		}
	}

	if buildController() {
		c := exec.Command("cp",
			filepath.Join("bazel-bin", "cmd", "manager", "manager_", "manager"),
			filepath.Join("bin", "manager"))
		klog.Infof("%s", strings.Join(c.Args, " "))
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err := c.Run()
		if err != nil {
			klog.Fatal(err)
		}
	}
}

func GoBuild(cmd *cobra.Command, args []string) {
	initApis()

	os.RemoveAll(filepath.Join("bin", "apiserver"))
	os.RemoveAll(filepath.Join("bin", "controller-manager"))

	if buildApiserver() {
		// Build the apiserver
		path := filepath.Join("cmd", "apiserver", "main.go")
		c := exec.Command("go", "build", "-o", filepath.Join(outputdir, "apiserver"), path)
		c.Env = append(os.Environ(), "CGO_ENABLED=0")
		klog.Infof("CGO_ENABLED=0")
		if len(goos) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("GOOS=%s", goos))
			klog.Infof(fmt.Sprintf("GOOS=%s", goos))
		}
		if len(goarch) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("GOARCH=%s", goarch))
			klog.Infof(fmt.Sprintf("GOARCH=%s", goarch))
		}

		klog.Infof("%s", strings.Join(c.Args, " "))
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err := c.Run()
		if err != nil {
			klog.Fatal(err)
		}
	}

	if buildController() {
		// Build the controller manager
		gocache := os.Getenv("GOCACHE")
		localAppData := os.Getenv("%LocalAppData%")
		path := filepath.Join("cmd", "manager", "main.go")
		c := exec.Command("go", "build", "-o", filepath.Join(outputdir, "controller-manager"), path)
		// add GOCACHE and LocalAppData environment variable
		if len(localAppData) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("GOCACHE=%s", gocache))
		}
		if len(localAppData) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("LocalAppData=%s", localAppData))
		}
		if len(os.Getenv("CGO_ENABLED")) == 0 {
			c.Env = append(os.Environ(), "CGO_ENABLED=0")
		}
		if len(goos) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("GOOS=%s", goos))
		}
		if len(goarch) > 0 {
			c.Env = append(c.Env, fmt.Sprintf("GOARCH=%s", goarch))
		}

		klog.Infof(strings.Join(c.Args, " "))
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err := c.Run()
		if err != nil {
			klog.Fatal(err)
		}
	}
}

func buildApiserver() bool {
	for _, t := range BuildTargets {
		if t == apiserverTarget {
			return true
		}
	}
	return false
}

func buildController() bool {
	for _, t := range BuildTargets {
		if t == controllerTarget {
			return true
		}
	}
	return false
}
