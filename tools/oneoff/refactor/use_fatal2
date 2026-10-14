#!/usr/bin/env ruby

Dir.glob("**/*.go").each do |file|
  next if file == "main.go" || file == "fatal.go" || file.end_with?("_test.go")
  
  content = File.read(file)
  changed = false

  # printError(...) \n os.Exit(1)
  new_content = content.gsub(/printError\((.*?)\)\n\s*os\.Exit\(1\)/m) do |match|
    changed = true
    "fatalError(\"%s\\n\", #{$1})"
  end
  
  if changed
    File.write(file, new_content)
  end
end
