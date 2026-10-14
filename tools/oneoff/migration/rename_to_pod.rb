#!/usr/bin/env ruby
# frozen_string_literal: true

require 'fileutils'

root_dir = File.expand_path('../..', __dir__)
Dir.chdir(root_dir)

# 1. Update go.mod
go_mod = File.join(root_dir, 'go.mod')
content = File.read(go_mod)
if content.start_with?("module abs\n")
  File.write(go_mod, content.sub(/\Amodule abs\n/, "module pod\n"))
  puts "Updated go.mod: module pod"
end

# 2. Update imports in all Go files
go_files = Dir.glob(File.join(root_dir, '**', '*.go')).reject do |f|
  f.include?('/.work/') || f.include?('/vendor/')
end

updated_count = 0
go_files.each do |file|
  src = File.read(file)
  if src.include?('"abs/pkg/')
    new_src = src.gsub('"abs/pkg/', '"pod/pkg/')
    File.write(file, new_src)
    updated_count += 1
  end
end
puts "Updated imports in #{updated_count} Go files."

# 3. Create cmd/pod/main.go
cmd_pod_dir = File.join(root_dir, 'cmd', 'pod')
FileUtils.mkdir_p(cmd_pod_dir)
cmd_pod_main = File.join(cmd_pod_dir, 'main.go')
File.write(cmd_pod_main, <<~GO)
package main

import (
	"os"

	"pod/pkg/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
GO
puts "Created cmd/pod/main.go"
