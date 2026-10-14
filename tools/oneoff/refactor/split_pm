#!/usr/bin/env ruby

lines = File.readlines("pm_download.go")
dl_lines = ["package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"strings\"\n\t\"time\"\n)\n\n"]
new_lines = []

lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 63 && idx <= 440
    dl_lines << line
  else
    new_lines << line
  end
end

File.write("pm_download_episodes.go", dl_lines.join)
File.write("pm_download.go", new_lines.join)
