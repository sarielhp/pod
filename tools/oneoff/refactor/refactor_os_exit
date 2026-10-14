#!/usr/bin/env ruby

content = File.read("profiles.go")

content.gsub!(/func setDefaultProfile\(cfg \*Config, targetID int\) \{/, "func setDefaultProfile(cfg *Config, targetID int) error {")
content.gsub!(/fmt\.Printf\("Default LLM profile updated to \\\[%d\\\] %s\\n", targetID, p\.Name\)\n\s+return\n\s+\}/, "fmt.Printf(\"Default LLM profile updated to [%d] %s\\n\", targetID, p.Name)\n\t\t\treturn nil\n\t\t}")
content.gsub!(/fmt\.Fprintf\(os\.Stderr, "Error: Profile ID \\\[%d\\\] not found in configuration\.\\n", targetID\)\n\s+os\.Exit\(1\)\n\}/, "return fmt.Errorf(\"Profile ID [%d] not found in configuration\", targetID)\n}")

content.gsub!(/func removeWhisperProfile\(cfg \*Config, targetID int\) \{/, "func removeWhisperProfile(cfg *Config, targetID int) error {")
content.gsub!(/fmt\.Fprintf\(os\.Stderr, "Error: Whisper server profile \\\[%d\\\] not found in configuration\.\\n", targetID\)\n\s+os\.Exit\(1\)/, "return fmt.Errorf(\"Whisper server profile [%d] not found in configuration\", targetID)")
content.gsub!(/fmt\.Printf\("Removed Whisper server profile \\\[%d\\\] %s\\n", targetID, profileName\)\n\}/, "fmt.Printf(\"Removed Whisper server profile [%d] %s\\n\", targetID, profileName)\n\treturn nil\n}")

content.gsub!(/func setDefaultWhisperProfile\(cfg \*Config, targetID int\) \{/, "func setDefaultWhisperProfile(cfg *Config, targetID int) error {")
content.gsub!(/fmt\.Println\("Default Whisper server updated to fallback\/legacy configuration\."\)\n\s+return/, "fmt.Println(\"Default Whisper server updated to fallback/legacy configuration.\")\n\t\treturn nil")
content.gsub!(/fmt\.Printf\("Default Whisper server profile updated to \\\[%d\\\] %s\\n", targetID, wp\.Name\)\n\s+return\n\s+\}/, "fmt.Printf(\"Default Whisper server profile updated to [%d] %s\\n\", targetID, wp.Name)\n\t\t\treturn nil\n\t\t}")
content.gsub!(/fmt\.Fprintf\(os\.Stderr, "Error: Whisper server profile \\\[%d\\\] not found in configuration\.\\n", targetID\)\n\s+os\.Exit\(1\)\n\}/, "return fmt.Errorf(\"Whisper server profile [%d] not found in configuration\", targetID)\n}")

File.write("profiles.go", content)
