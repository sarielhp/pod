#!/usr/bin/env ruby

lines = File.readlines("transcribe.go")
chunks_lines = ["package main\n\nimport (\n\t\"bytes\"\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"io\"\n\t\"mime/multipart\"\n\t\"net/http\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"strconv\"\n)\n\n"]
new_lines = []

lines.each_with_index do |line, i|
  idx = i + 1
  if idx >= 170 && idx <= 394
    chunks_lines << line
  else
    new_lines << line
  end
end

File.write("transcribe_chunks.go", chunks_lines.join)
File.write("transcribe.go", new_lines.join)
