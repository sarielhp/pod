#!/usr/bin/env ruby
# frozen_string_literal: true

require 'fileutils'

caddy_compose = '/opt/stacks/caddy/docker-compose.yml'
clean_dir = '/media/podcasts/clean'
abs_dir = '/media/podcasts/abs'
podfetch_link = '/media/podcasts/podfetch'

puts "=== Step 1: Updating Caddy Docker Compose Mount ==="

if File.exist?(caddy_compose)
  content = File.read(caddy_compose)
  if content.include?('/media/podcasts/abs:/srv/podcasts:ro')
    backup_file = "#{caddy_compose}.bak.#{Time.now.strftime('%Y%m%d%H%M%S')}"
    FileUtils.cp(caddy_compose, backup_file)
    puts "  Created backup: #{backup_file}"

    updated = content.gsub(%r{/media/podcasts/abs:/srv/podcasts:ro}, '/media/podcasts/clean:/srv/podcasts:ro')
    File.write(caddy_compose, updated)
    puts "  Updated #{caddy_compose} mount to /media/podcasts/clean:/srv/podcasts:ro"
  elsif content.include?('/media/podcasts/clean:/srv/podcasts:ro')
    puts "  #{caddy_compose} already configured with /media/podcasts/clean"
  else
    puts "  Warning: Did not find matching /media/podcasts mount in #{caddy_compose}"
  end

  puts "\n=== Step 2: Restarting Caddy Container ==="
  cmd = "docker compose -f #{caddy_compose} up -d"
  puts "  Running: #{cmd}"
  success = system(cmd)
  if success
    puts "  Caddy restarted successfully."
  else
    warn "  Warning: Docker compose restart failed (exit code #{$?.exitstatus}). Please check permissions or run with sudo."
  end
else
  warn "  Warning: #{caddy_compose} not found, skipping Caddy update."
end

puts "\n=== Step 3: Removing Legacy Podcasts Directories ==="

# Safety check: ensure clean_dir exists and has podcasts before deleting abs_dir
unless Dir.exist?(clean_dir) && Dir.glob("#{clean_dir}/*").size > 10
  abort "Error: Safety guard triggered! #{clean_dir} does not exist or has too few files. Aborting deletion."
end

if File.symlink?(podfetch_link) || File.exist?(podfetch_link)
  FileUtils.rm_f(podfetch_link)
  puts "  Removed symlink: #{podfetch_link}"
else
  puts "  Symlink #{podfetch_link} already removed."
end

if Dir.exist?(abs_dir)
  puts "  Removing legacy directory: #{abs_dir} (this may take a moment)..."
  FileUtils.rm_rf(abs_dir)
  puts "  Successfully removed #{abs_dir}."
else
  puts "  Directory #{abs_dir} already removed."
end

puts "\n=== Cleanup Complete ==="
puts "All podcasts are now served exclusively from #{clean_dir}."
