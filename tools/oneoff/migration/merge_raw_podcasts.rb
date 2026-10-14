#!/usr/bin/env ruby
# frozen_string_literal: true

require 'fileutils'

clean_dir = '/media/podcasts/clean'
raw_dir = '/media/podcasts/raw'

puts "=== Step 1: Merging Missing Episodes from #{raw_dir} into #{clean_dir} ==="

unless Dir.exist?(raw_dir)
  puts "Notice: #{raw_dir} does not exist. Nothing to merge."
  exit 0
end

unless Dir.exist?(clean_dir) && Dir.glob("#{clean_dir}/*").size > 10
  abort "Error: Safety guard failed. Destination #{clean_dir} does not exist or has too few items."
end

copied_count = 0
skipped_count = 0
copied_bytes = 0
errors = []

Dir.glob("#{raw_dir}/*/*/podcast.mp3").sort.each do |src_mp3|
  show = File.basename(File.dirname(File.dirname(src_mp3)))
  ep_title = File.basename(File.dirname(src_mp3))

  dest_show_dir = File.join(clean_dir, show)
  FileUtils.mkdir_p(dest_show_dir)

  dest_mp3 = File.join(dest_show_dir, "#{ep_title}.mp3")

  if File.exist?(dest_mp3)
    skipped_count += 1
    next
  end

  begin
    FileUtils.cp(src_mp3, dest_mp3)
    copied_count += 1
    copied_bytes += File.size(dest_mp3)
    puts "  [#{show}] Imported: #{ep_title}"

    # Also copy HTML report if present
    report_src = File.join(File.dirname(src_mp3), 'podcast_cut.report.html')
    dest_report = File.join(dest_show_dir, "#{ep_title}.report.html")
    FileUtils.cp(report_src, dest_report) if File.exist?(report_src) && !File.exist?(dest_report)
  rescue StandardError => e
    errors << "Failed to copy #{src_mp3}: #{e.message}"
  end
end

puts "\nSummary of Copy Stage:"
puts "  Copied:  #{copied_count} episode(s) (#{(copied_bytes / (1024.0 * 1024 * 1024)).round(2)} GB)"
puts "  Skipped: #{skipped_count} episode(s) (already present in clean)"

if errors.any?
  warn "\nErrors encountered during copy:"
  errors.each { |err| warn "  #{err}" }
  abort "Aborting deletion of #{raw_dir} due to copy errors."
end

puts "\n=== Step 2: Removing #{raw_dir} ==="
FileUtils.rm_rf(raw_dir)
puts "  Successfully removed #{raw_dir}."

puts "\n=== Step 3: Regenerating Feeds and Catalog ==="
abs_binary = File.expand_path('../../abs', __dir__)
if File.exist?(abs_binary) && File.executable?(abs_binary)
  puts "  Running: #{abs_binary} server feed"
  system(abs_binary, 'server', 'feed')
else
  puts "  Note: Run 'abs server feed' to refresh RSS feeds and web catalog."
end

puts "\n=== Raw Merge Complete ==="
