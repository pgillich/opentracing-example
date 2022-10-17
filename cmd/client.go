/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
)

// clientCmd represents the client command
var clientCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "client",
	Short: "Client",
	Long:  `Client command`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		return RunService(cmd, args, &internal.ClientConfig{}, internal.NewClientService)
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().String("server", "localhost:8882", "FE server address")
	if err := viper.BindPFlags(clientCmd.Flags()); err != nil {
		logger.GetLogger(clientCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
