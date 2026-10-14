#!/usr/bin/env ruby

lines = File.readlines("tui_keys_test.go")
extra_lines = ["package main\n\nimport (\n\t\"strings\"\n\t\"testing\"\n\t\"time\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n)\n\n"]
new_lines = []

lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 294 && idx <= 538
    extra_lines << line
  else
    new_lines << line
  end
end

File.write("tui_keys_extra_test.go", extra_lines.join)
File.write("tui_keys_test.go", new_lines.join)
