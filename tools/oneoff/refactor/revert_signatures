#!/usr/bin/env ruby

content = File.read("profiles.go")
content.gsub!(/func setDefaultProfile\(cfg \*Config, targetID int\) error \{/, "func setDefaultProfile(cfg *Config, targetID int) {")
content.gsub!(/func removeWhisperProfile\(cfg \*Config, targetID int\) error \{/, "func removeWhisperProfile(cfg *Config, targetID int) {")
content.gsub!(/func setDefaultWhisperProfile\(cfg \*Config, targetID int\) error \{/, "func setDefaultWhisperProfile(cfg *Config, targetID int) {")

File.write("profiles.go", content)
