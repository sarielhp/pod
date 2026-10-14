#!/usr/bin/env ruby

lines = File.readlines("player.go")
sink_lines = ["package main\n\nimport (\n\t\"fmt\"\n\t\"os/exec\"\n\t\"strconv\"\n\t\"strings\"\n)\n\n"]
new_lines = []

lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 293 && idx <= 420
    sink_lines << line
  else
    new_lines << line
  end
end

File.write("player_sink.go", sink_lines.join)
File.write("player.go", new_lines.join)
