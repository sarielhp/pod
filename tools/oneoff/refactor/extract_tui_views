#!/usr/bin/env ruby

lines = File.readlines("tui.go")
tui_views_lines = ["package main\n", "import (\n", "\t\"strings\"\n", ")\n\n"]
new_tui_lines = []

in_extract = false
lines.each_with_index do |line, i|
  idx = i + 1
  if (idx >= 350 && idx <= 355) || (idx >= 392 && idx <= 403) || (idx >= 426 && idx <= 500)
    tui_views_lines << line
  else
    new_tui_lines << line
  end
end

File.write("tui_views.go", tui_views_lines.join)
File.write("tui.go", new_tui_lines.join)
