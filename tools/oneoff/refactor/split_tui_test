#!/usr/bin/env ruby

lines = File.readlines("tui_test.go")
nav_lines = ["package main\n\nimport (\n\t\"testing\"\n\n\ttea \"github.com/charmbracelet/bubbletea\"\n)\n\n"]
new_lines = []

lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 267 && idx <= 540
    nav_lines << line
  else
    new_lines << line
  end
end

File.write("tui_nav_test.go", nav_lines.join)
File.write("tui_test.go", new_lines.join)
