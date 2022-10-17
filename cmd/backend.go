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

// backendCmd represents the backend command
var backendCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "backend",
	Short: "Backend",
	Long:  `Backend server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		return RunService(cmd, args, &internal.BackendConfig{}, internal.NewBackendService)
	},
}

func init() {
	rootCmd.AddCommand(backendCmd)
	backendCmd.Flags().String("listenaddr", "localhost:8881", "Listen address")
	backendCmd.Flags().String("response", "Hello", "Response text")
	if err := viper.BindPFlags(backendCmd.Flags()); err != nil {
		logger.GetLogger(backendCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
