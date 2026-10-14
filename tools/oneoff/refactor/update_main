#!/usr/bin/env ruby

content = File.read("main.go")

content.gsub!(/setDefaultProfile\(&config, id\)/, "if err := setDefaultProfile(&config, id); err != nil {\n\t\t\t\t\tfmt.Fprintf(os.Stderr, \"Error: %v\\n\", err)\n\t\t\t\t\tos.Exit(1)\n\t\t\t\t}")
content.gsub!(/setDefaultWhisperProfile\(&config, id\)/, "if err := setDefaultWhisperProfile(&config, id); err != nil {\n\t\t\t\t\tfmt.Fprintf(os.Stderr, \"Error: %v\\n\", err)\n\t\t\t\t\tos.Exit(1)\n\t\t\t\t}")
content.gsub!(/removeWhisperProfile\(&config, id\)/, "if err := removeWhisperProfile(&config, id); err != nil {\n\t\t\t\t\tfmt.Fprintf(os.Stderr, \"Error: %v\\n\", err)\n\t\t\t\t\tos.Exit(1)\n\t\t\t\t}")

File.write("main.go", content)
