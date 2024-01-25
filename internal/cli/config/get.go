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

package config

import (
	"os"

	"github.com/arduino/arduino-cli/commands/daemon"
	"github.com/arduino/arduino-cli/internal/cli/configuration"
	"github.com/arduino/arduino-cli/internal/cli/feedback"
	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func initGetCommand() *cobra.Command {
	getCommand := &cobra.Command{
		Use:   "get",
		Short: tr("Gets settings key values."),
		Long:  tr("Gets settings key values."),
		Example: "" +
			"  " + os.Args[0] + " config get logging\n" +
			"  " + os.Args[0] + " config get logging.level logging.file\n" +
			"  " + os.Args[0] + " config get board_manager.additional_urls",
		Args: cobra.MinimumNArgs(1),
		Run:  runGetCommand,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return configuration.Settings.AllKeys(), cobra.ShellCompDirectiveDefault
		},
	}
	return getCommand
}

func runGetCommand(cmd *cobra.Command, args []string) {
	logrus.Info("Executing `arduino-cli config get`")

	svc := daemon.ArduinoCoreServerImpl{}
	for _, toGet := range args {
		resp, err := svc.SettingsGetValue(cmd.Context(), &rpc.SettingsGetValueRequest{Key: toGet})
		if err != nil {
			feedback.Fatal(tr("Cannot get the key %[1]s: %[2]v", toGet, err), feedback.ErrGeneric)
		}
		feedback.PrintResult(getResult{key: toGet, data: resp.GetJsonData()})
	}
}

// output from this command may require special formatting.
// create a dedicated feedback.Result implementation to safely handle
// any changes to the configuration.Settings struct.
type getResult struct {
	key  string
	data interface{}
}

func (gr getResult) Data() interface{} {
	return gr.data
}

func (gr getResult) String() string {
	gs, ok := gr.data.(string)
	if !ok {
		// Should never happen
		panic(tr("Cannot get key %s value as string: %v", gr.key, gr.data))
	}
	return gs
}
