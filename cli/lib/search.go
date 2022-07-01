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

package lib

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/arduino/arduino-cli/cli/errorcodes"
	"github.com/arduino/arduino-cli/cli/feedback"
	"github.com/arduino/arduino-cli/cli/instance"
	"github.com/arduino/arduino-cli/cli/output"
	"github.com/arduino/arduino-cli/commands"
	"github.com/arduino/arduino-cli/commands/lib"
	"github.com/arduino/arduino-cli/configuration"
	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
	"github.com/arduino/go-paths-helper"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	semver "go.bug.st/relaxed-semver"
)

var (
	namesOnly bool // if true outputs lib names only.
)

func initSearchCommand() *cobra.Command {
	searchCommand := &cobra.Command{
		Use:     fmt.Sprintf("search [%s]", tr("LIBRARY_NAME")),
		Short:   tr("Searches for one or more libraries data."),
		Long:    tr("Search for one or more libraries data (case insensitive search)."),
		Example: "  " + os.Args[0] + " lib search audio",
		Args:    cobra.ArbitraryArgs,
		Run:     runSearchCommand,
	}
	searchCommand.Flags().BoolVar(&namesOnly, "names", false, tr("Show library names only."))
	return searchCommand
}

// indexUpdateInterval specifies the time threshold over which indexes are updated
const indexUpdateInterval = "60m"

func runSearchCommand(cmd *cobra.Command, args []string) {
	inst, status := instance.Create()
	logrus.Info("Executing `arduino-cli lib search`")

	if status != nil {
		feedback.Errorf(tr("Error creating instance: %v"), status)
		os.Exit(errorcodes.ErrGeneric)
	}

	if indexNeedsUpdating(indexUpdateInterval) {
		if err := commands.UpdateLibrariesIndex(
			context.Background(),
			&rpc.UpdateLibrariesIndexRequest{Instance: inst},
			output.ProgressBar(),
		); err != nil {
			feedback.Errorf(tr("Error updating library index: %v"), err)
			os.Exit(errorcodes.ErrGeneric)
		}
	}

	for _, err := range instance.Init(inst) {
		feedback.Errorf(tr("Error initializing instance: %v"), err)
	}

	searchResp, err := lib.LibrarySearch(context.Background(), &rpc.LibrarySearchRequest{
		Instance: inst,
		Query:    (strings.Join(args, " ")),
	})
	if err != nil {
		feedback.Errorf(tr("Error searching for Library: %v"), err)
		os.Exit(errorcodes.ErrGeneric)
	}

	feedback.PrintResult(result{
		results:   searchResp,
		namesOnly: namesOnly,
	})

	logrus.Info("Done")
}

// output from this command requires special formatting, let's create a dedicated
// feedback.Result implementation
type result struct {
	results   *rpc.LibrarySearchResponse
	namesOnly bool
}

func (res result) Data() interface{} {
	if res.namesOnly {
		type LibName struct {
			Name string `json:"name"`
		}

		type NamesOnly struct {
			Libraries []LibName `json:"libraries"`
		}

		names := []LibName{}
		results := res.results.GetLibraries()
		for _, lib := range results {
			names = append(names, LibName{lib.Name})
		}

		return NamesOnly{
			names,
		}
	}

	return res.results
}

func (res result) String() string {
	results := res.results.GetLibraries()
	if len(results) == 0 {
		return tr("No libraries matching your search.")
	}

	// get a sorted slice of results
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	var out strings.Builder

	if res.results.GetStatus() == rpc.LibrarySearchStatus_LIBRARY_SEARCH_STATUS_FAILED {
		out.WriteString(tr("No libraries matching your search.\nDid you mean...\n"))
	}

	for _, lib := range results {
		if res.results.GetStatus() == rpc.LibrarySearchStatus_LIBRARY_SEARCH_STATUS_SUCCESS {
			out.WriteString(tr(`Name: "%s"`, lib.Name) + "\n")
			if res.namesOnly {
				continue
			}
		} else {
			out.WriteString(fmt.Sprintf("%s\n", lib.Name))
			continue
		}

		latest := lib.GetLatest()

		deps := []string{}
		for _, dep := range latest.GetDependencies() {
			if dep.GetVersionConstraint() == "" {
				deps = append(deps, dep.GetName())
			} else {
				deps = append(deps, dep.GetName()+" ("+dep.GetVersionConstraint()+")")
			}
		}

		out.WriteString(fmt.Sprintf("  "+tr("Author: %s")+"\n", latest.Author))
		out.WriteString(fmt.Sprintf("  "+tr("Maintainer: %s")+"\n", latest.Maintainer))
		out.WriteString(fmt.Sprintf("  "+tr("Sentence: %s")+"\n", latest.Sentence))
		out.WriteString(fmt.Sprintf("  "+tr("Paragraph: %s")+"\n", latest.Paragraph))
		out.WriteString(fmt.Sprintf("  "+tr("Website: %s")+"\n", latest.Website))
		if latest.License != "" {
			out.WriteString(fmt.Sprintf("  "+tr("License: %s")+"\n", latest.License))
		}
		out.WriteString(fmt.Sprintf("  "+tr("Category: %s")+"\n", latest.Category))
		out.WriteString(fmt.Sprintf("  "+tr("Architecture: %s")+"\n", strings.Join(latest.Architectures, ", ")))
		out.WriteString(fmt.Sprintf("  "+tr("Types: %s")+"\n", strings.Join(latest.Types, ", ")))
		out.WriteString(fmt.Sprintf("  "+tr("Versions: %s")+"\n", strings.Replace(fmt.Sprint(versionsFromSearchedLibrary(lib)), " ", ", ", -1)))
		if len(latest.ProvidesIncludes) > 0 {
			out.WriteString(fmt.Sprintf("  "+tr("Provides includes: %s")+"\n", strings.Join(latest.ProvidesIncludes, ", ")))
		}
		if len(latest.Dependencies) > 0 {
			out.WriteString(fmt.Sprintf("  "+tr("Dependencies: %s")+"\n", strings.Join(deps, ", ")))
		}
	}

	return out.String()
}

func versionsFromSearchedLibrary(library *rpc.SearchedLibrary) []*semver.Version {
	res := []*semver.Version{}
	for str := range library.Releases {
		if v, err := semver.Parse(str); err == nil {
			res = append(res, v)
		}
	}
	sort.Sort(semver.List(res))
	return res
}

// indexNeedsUpdating returns whether library_index.json need updating.
// A positive duration string must be provided to calculate the time threshold
// used to update the index.
// Valid duration units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
// Use a duration of 0 to always update the index.
func indexNeedsUpdating(duration string) bool {
	// Library index path is constant (relative to the data directory).
	// It does not depend on board manager URLs or any other configuration.
	dataDir := configuration.Settings.GetString("directories.Data")
	indexPath := paths.New(dataDir).Join("library_index.json")
	// Verify the index file exists and we can read its fstat attrs.
	if indexPath.NotExist() {
		return true
	}
	info, err := indexPath.Stat()
	if err != nil {
		return true
	}
	// Sanity check the given threshold duration string.
	now := time.Now()
	modTimeThreshold, err := time.ParseDuration(duration)
	if err != nil {
		feedback.Error(tr("Invalid timeout: %s", err))
		os.Exit(errorcodes.ErrBadArgument)
	}
	// The behavior of now.After(T) is confusing if T < 0 and MTIME in the future,
	// and is probably not what the user intended. Disallow negative T and inform
	// the user that positive thresholds are expected.
	if modTimeThreshold < 0 {
		feedback.Error(tr("Timeout must be non-negative: %dns (%s)", modTimeThreshold, duration))
		os.Exit(errorcodes.ErrBadArgument)
	}
	return modTimeThreshold == 0 || now.After(info.ModTime().Add(modTimeThreshold))
}
