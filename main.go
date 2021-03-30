package main

import (
	"log"
	"os"

	"github.com/gonejack/mhtml-to-html/cmd"
	"github.com/spf13/cobra"
)

var (
	verbose = false

	prog = &cobra.Command{
		Use:   "mhtml-to-html *.mht",
		Short: "Command line tool for converting mhtml to html.",
		Run: func(c *cobra.Command, args []string) {
			err := run(c, args)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
)

func init() {
	log.SetOutput(os.Stdout)

	prog.Flags().SortFlags = false
	prog.PersistentFlags().SortFlags = false

	prog.PersistentFlags().BoolVarP(
		&verbose,
		"verbose",
		"v",
		false,
		"verbose",
	)
}

func run(c *cobra.Command, args []string) error {
	exec := cmd.MHTMLToHTML{
		ImagesDir: "images",
		Verbose:   verbose,
	}

	return exec.Run(args)
}

func main() {
	_ = prog.Execute()
}
