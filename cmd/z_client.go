/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"time"

	"github.com/sagikazarmark/slog-shim"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

var clientViper = viper.New() //nolint:gochecknoglobals // CMD

// clientCmd represents the client command
var clientCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "client",
	Short: "Client",
	Long:  `Client command`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())

		err := RunService(cmd.Context(), cmd.Use, args, clientViper, &internal.ClientConfig{
			Command: fmt.Sprintf("%+v", cmd.Context().Value(model.CtxKeyCmd)),
		}, internal.NewClientService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().String("server", "localhost:8882", "FE server address")
	clientCmd.Flags().String("instance", "#3", "Client instance")
	clientCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector address")
	if err := clientViper.BindPFlags(clientCmd.Flags()); err != nil {
		logger.GetLogger(clientCmd.Use, slog.LevelDebug).Error("Unable to bind flags", logger.KeyError, err)
		panic(err)
	}
	clientViper.AutomaticEnv()
}
