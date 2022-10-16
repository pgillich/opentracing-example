/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"context"
	"os"
	"strings"

	"emperror.dev/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

var cfgFile string //nolint:gochecknoglobals // cobra

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{ //nolint:gochecknoglobals // cobra
	Use:   "opentracing-example",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context, args []string, serverRunner model.ServerRunner) {
	ctx = context.WithValue(ctx, model.CtxKeyCmd, strings.Join(append([]string{rootCmd.Use}, args...), " "))
	ctx = context.WithValue(ctx, model.CtxKeyServerRunner, serverRunner)
	rootCmd.SetArgs(args)
	rootCmd.SetContext(ctx)
	if err := rootCmd.Execute(); err != nil {
		logger.GetLogger(rootCmd.Use).Error(err, "Bad", "args", args)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.opentracing-example.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".opentracing-example")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logger.GetLogger(rootCmd.Use).Info("Using config file", "path", viper.ConfigFileUsed())
	}
}

func RunService(cmd *cobra.Command, args []string, config interface{}, newService model.NewService) error {
	commandLine := cmd.Context().Value(model.CtxKeyCmd)
	log := logger.GetLogger(cmd.Use).WithValues(logger.KeyCmd, commandLine)

	if err := viper.Unmarshal(config); err != nil {
		return err
	}

	return errors.Wrap(newService(cmd.Context(), config, log).Run(args), "service run")
}
