package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts the API server",
	Run: func(cmd *cobra.Command, args []string) {
		startServer()
	},
}

func startServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Welcome to the API server!")
	})
	setupRoutes()
	port := viper.GetInt("port")
	log.Printf("Starting server on :%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func setupRoutes() {
	http.HandleFunc("/do-task", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// generateCmd.Flags().StringVarP(&config.GenerateChartUrl, "chart", "c", "", lang.CmdGenerateFlagChart)
		// generateCmd.Flags().StringVarP(&config.GenerateChartName, "name", "n", "", lang.CmdGenerateFlagName)
		// generateCmd.Flags().StringVarP(&config.GenerateChartVersion, "version", "v", "", lang.CmdGenerateFlagVersion)
		// generateCmd.Flags().StringVarP(&config.GenerateOutputDir, "output", "o", "generated", lang.CmdGenerateOutputDir)
		// Set configuration from the form values
		for key, values := range r.Form {
			if len(values) > 0 {
				viper.Set(key, values[0]) // Assuming the first value is the desired one
			}
		}
		// taskParams := map[string]interface{}{
		// 	"param1": r.FormValue("param1"),
		// 	"param2": r.FormValue("param2"),
		// 	"param3": r.FormValue("param3"),
		// }

		// Simulate the task performance as if it was a CLI call
		// result, err := generate.Generate()
		// if err != nil {
		// 	http.Error(w, err.Error(), http.StatusInternalServerError)
		// 	return
		// }

		// response, _ := json.Marshal(result)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("We did the thing!"))
	})
}

func init() {
	rootCmd.AddCommand(startCmd)
	startCmd.Flags().IntP("port", "p", 8080, "Port on which the server will run")
	viper.BindPFlag("port", startCmd.Flags().Lookup("port"))
}
