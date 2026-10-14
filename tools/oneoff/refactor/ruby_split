#!/usr/bin/env ruby

src = ARGV[0]
dst = ARGV[1]
pattern = Regexp.new(ARGV[2], Regexp::MULTILINE)

code = File.read(src)
matches = code.scan(pattern)

unless matches.empty?
  dst_code = "package main\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\n"
  matches.each do |m|
    dst_code += m[0] + "\n\n"
  end
  File.write(dst, dst_code)
  
  new_code = code.gsub(pattern, "")
  File.write(src, new_code)
  puts "Moved \#{matches.size} blocks from \#{src} to \#{dst}"
else
  puts "No matches found"
end
