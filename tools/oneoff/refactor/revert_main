#!/usr/bin/env ruby

content = File.read("main.go")
content.gsub!(/if err := setDefaultProfile\(&config, id\); err != nil \{\n\t\t\t\t\tfmt\.Fprintf\(os\.Stderr, "Error: %v\\n", err\)\n\t\t\t\t\tos\.Exit\(1\)\n\t\t\t\t\}/, "setDefaultProfile(&config, id)")
content.gsub!(/if err := setDefaultWhisperProfile\(&config, id\); err != nil \{\n\t\t\t\t\tfmt\.Fprintf\(os\.Stderr, "Error: %v\\n", err\)\n\t\t\t\t\tos\.Exit\(1\)\n\t\t\t\t\}/, "setDefaultWhisperProfile(&config, id)")
content.gsub!(/if err := removeWhisperProfile\(&config, id\); err != nil \{\n\t\t\t\t\tfmt\.Fprintf\(os\.Stderr, "Error: %v\\n", err\)\n\t\t\t\t\tos\.Exit\(1\)\n\t\t\t\t\}/, "removeWhisperProfile(&config, id)")

File.write("main.go", content)
