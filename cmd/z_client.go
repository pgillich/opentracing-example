/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

var (
	ClientViper *viper.Viper //nolint:gochecknoglobals // demo
)

// clientCmd represents the client command
var clientCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "client",
	Short: "Client",
	Long:  `Client command`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SetContext(cmd.Parent().Context())
		initConfigViper(ClientViper)

		err := RunService(cmd.Context(), cmd.Use, args, &internal.ClientConfig{
			Command: fmt.Sprintf("%+v", cmd.Context().Value(model.CtxKeyCmd)),
		}, ClientViper, internal.NewClientService)
		time.Sleep(time.Second)

		return err
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().String("server", "localhost:8882", "FE server address")
	clientCmd.Flags().String("instance", "#3", "Client instance")
	clientCmd.Flags().String("jaegerURL", "http://localhost:14268/api/traces", "Jaeger collector URL")
	ClientViper = viper.New()
	if err := ClientViper.BindPFlags(clientCmd.Flags()); err != nil {
		logger.GetLogger(clientCmd.Use).Error(err, "Unable to bind flags")
		panic(err)
	}
}
