#!/usr/bin/env ruby

lines = File.readlines("tui.go")
tui_types_lines = ["package main\n", "import (\n", "\t\"fmt\"\n", "\t\"time\"\n", ")\n\n"]
new_tui_lines = []

in_extract = false
lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 27 && idx <= 117
    tui_types_lines << line
  else
    new_tui_lines << line
  end
end

File.write("tui_types.go", tui_types_lines.join)
File.write("tui.go", new_tui_lines.join)
