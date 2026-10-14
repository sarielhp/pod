#!/usr/bin/env ruby

Dir.glob("**/*.go").each do |file|
  next if file.include?("vendor/")
  
  content = File.read(file)
  new_lines = []
  
  in_block_comment = false
  
  content.each_line do |line|
    if in_block_comment
      if line.include?('*/')
        in_block_comment = false
      end
      next
    end
    
    if line.match?(/^\s*\/\*/)
      in_block_comment = true
      if line.include?('*/')
        in_block_comment = false
      end
      next
    end
    
    # Exclude //go:
    if line.match?(/^\s*\/\/(?!go:)/)
      next
    end
    
    # Handle inline comments, but be careful with URLs in strings.
    # We will only remove full-line comments as that covers 99% of them in this codebase.
    new_lines << line
  end
  
  File.write(file, new_lines.join)
end
