#!/usr/bin/env ruby
# frozen_string_literal: true

require 'minitest/autorun'
require 'tmpdir'
require 'fileutils'
require 'open3'
require 'rbconfig'
require 'digest'

load File.expand_path('install', __dir__)

class ReleaseWorkflowTest < Minitest::Test
  def setup
    work_dir = File.expand_path('../.work', __dir__)
    FileUtils.mkdir_p(work_dir)
    @fixture = Dir.mktmpdir('release-test-', work_dir)
    @repo = File.join(@fixture, 'repo')
    @log = File.join(@fixture, 'commands.log')
    FileUtils.mkdir_p(File.join(@repo, 'tools'))
    File.write(File.join(@repo, 'VERSION'), "0.1.0\n")
    @environment = { 'POD_WORKFLOW_TEST_LOG' => @log }
  end

  def teardown
    FileUtils.rm_rf(@fixture)
  end

  def copy_tool(name)
    destination = File.join(@repo, 'tools', name)
    FileUtils.cp(File.join(__dir__, name), destination)
    File.chmod(0o755, destination)
  end

  def write_executable(path, body)
    File.write(path, "#!#{RbConfig.ruby}\n#{body}\n")
    File.chmod(0o755, path)
  end

  def stub_stage(name, exit_status = 0)
    body = "File.open(ENV.fetch('POD_WORKFLOW_TEST_LOG'), 'a') { |file| file.puts #{name.inspect} }; exit #{exit_status}"
    write_executable(File.join(@repo, 'tools', name), body)
  end

  def run_tool(name, *)
    Open3.capture3(@environment, RbConfig.ruby, File.join(@repo, 'tools', name), *)
  end

  def git(*)
    stdout, stderr, status = Open3.capture3('git', '-C', @repo, *)
    assert status.success?, stderr
    stdout.strip
  end

  def prepare_bump
    copy_tool('bump')
    stub_stage('check')
    File.write(File.join(@repo, '.gitignore'), ".verified_head\n")
    git('init', '-q', '-b', 'main')
    git('config', 'user.email', 'release-test@example.invalid')
    git('config', 'user.name', 'Release Test')
    git('config', 'commit.gpgsign', 'false')
    git('config', 'core.hooksPath', File.join(@repo, '.git', 'hooks'))
    git('add', '.')
    git('commit', '-qm', 'initial')
    @remote = File.join(@fixture, 'remote.git')
    git('init', '-q', '--bare', '-b', 'main', @remote)
    git('--git-dir', @remote, 'config', 'core.hooksPath', File.join(@remote, 'hooks'))
    git('remote', 'add', 'origin', @remote)
    git('push', '-qu', 'origin', 'main')
  end

  def test_release_order_and_install
    [false, true].each do |install|
      copy_tool('release')
      %w[commit bump install].each { |stage| stub_stage(stage) }
      arguments = install ? ['--install', 'fix: release test'] : ['fix: release test']
      stdout, stderr, status = run_tool('release', *arguments)
      assert status.success?, stderr
      assert_includes stdout, 'Released 0.1.0 and installed'
      assert_equal %w[commit bump install], File.readlines(@log, chomp: true)
      FileUtils.rm_f(@log)
    end
  end

  def test_release_stops_at_each_failed_stage
    %w[commit bump install].each_with_index do |failed_stage, index|
      copy_tool('release')
      %w[commit bump install].each { |stage| stub_stage(stage, stage == failed_stage ? 1 : 0) }
      stdout, stderr, status = run_tool('release', 'fix: release test')
      refute status.success?
      refute_includes stdout, 'Released '
      refute_empty stderr
      assert_equal %w[commit bump install].first(index + 1), File.readlines(@log, chomp: true)
      FileUtils.rm_f(@log)
    end
  end

  def test_release_requires_message_before_mutating
    copy_tool('release')
    stub_stage('commit')
    _, stderr, status = run_tool('release')
    refute status.success?
    assert_includes stderr, 'Usage:'
    refute File.exist?(@log)
  end

  def test_release_runs_real_git_workflow_with_one_gate
    prepare_bump
    copy_tool('release')
    copy_tool('commit')
    stub_stage('install')
    File.write(File.join(@repo, 'change.txt'), 'release change')
    stdout, stderr, status = run_tool('release', 'fix: release change')
    assert status.success?, stderr
    assert_includes stdout, 'Released 0.1.1 and installed'
    assert_equal %w[check install], File.readlines(@log, chomp: true)
    assert_equal git('rev-parse', 'HEAD'), git('ls-remote', 'origin', 'refs/heads/main').split.first
    assert_empty git('status', '--porcelain')
  end

  def test_bump_verifies_remote_and_skips_verified_gate
    prepare_bump
    File.write(File.join(@repo, '.verified_head'), "#{git('rev-parse', 'HEAD')}\n")
    stdout, stderr, status = run_tool('bump')
    assert status.success?, stderr
    assert_includes stdout, 'Success 0.1.1'
    assert_equal '0.1.1', File.read(File.join(@repo, 'VERSION')).strip
    remote_sha = git('ls-remote', 'origin', 'refs/heads/main').split.first
    assert_equal git('rev-parse', 'HEAD'), remote_sha
    refute File.exist?(File.join(@repo, '.verified_head'))
    refute File.exist?(@log)
  end

  def test_bump_fails_before_mutation_without_upstream
    prepare_bump
    git('config', '--unset', 'branch.main.remote')
    stdout, _, status = run_tool('bump')
    refute status.success?
    refute_includes stdout, 'Success'
    assert_equal "0.1.0\n", File.read(File.join(@repo, 'VERSION'))
  end

  def test_bump_stops_on_staging_failure
    prepare_bump
    previous_head = git('rev-parse', 'HEAD')
    File.write(File.join(@repo, '.git', 'index.lock'), '')
    stdout, stderr, status = run_tool('bump')
    refute status.success?
    assert_includes stderr, 'git add VERSION'
    refute_includes stdout, 'Success'
    assert_equal previous_head, git('rev-parse', 'HEAD')
    assert_equal previous_head, git('ls-remote', 'origin', 'refs/heads/main').split.first
  end

  def test_bump_stops_on_commit_failure
    prepare_bump
    previous_head = git('rev-parse', 'HEAD')
    write_executable(File.join(@repo, '.git', 'hooks', 'pre-commit'), 'abort "commit rejected"')
    stdout, stderr, status = run_tool('bump')
    refute status.success?
    assert_includes stderr, 'git commit'
    refute_includes stdout, 'Success'
    assert_equal previous_head, git('rev-parse', 'HEAD')
    assert_equal previous_head, git('ls-remote', 'origin', 'refs/heads/main').split.first
  end

  def test_bump_reports_rejected_push_and_retains_local_commit
    prepare_bump
    previous_head = git('rev-parse', 'HEAD')
    write_executable(File.join(@remote, 'hooks', 'pre-receive'), 'abort "push rejected"')
    stdout, stderr, status = run_tool('bump')
    refute status.success?
    assert_includes stderr, 'git push'
    refute_includes stdout, 'Success'
    refute_equal previous_head, git('rev-parse', 'HEAD')
    assert_equal previous_head, git('ls-remote', 'origin', 'refs/heads/main').split.first
  end

  def test_bump_rejects_remote_verification_mismatch
    prepare_bump
    real_git = Open3.capture3('which', 'git').first.strip
    fake_bin = File.join(@fixture, 'fake-bin')
    FileUtils.mkdir_p(fake_bin)
    fake_remote = ('0' * 40) + "\trefs/heads/main"
    body = "if ARGV.first == 'ls-remote'; puts #{fake_remote.inspect}; else; exec #{real_git.inspect}, *ARGV; end"
    write_executable(File.join(fake_bin, 'git'), body)
    @environment['PATH'] = "#{fake_bin}:#{ENV.fetch('PATH')}"
    stdout, stderr, status = run_tool('bump')
    refute status.success?
    assert_includes stderr, 'Push verification failed'
    refute_includes stdout, 'Success'
  end

  def test_install_verifies_binary_before_removing_legacy
    source = File.join(@fixture, 'pod')
    install_dir = File.join(@fixture, 'bin')
    FileUtils.mkdir_p(install_dir)
    File.binwrite(source, "test binary\x00\xff")
    File.write(File.join(install_dir, 'abs'), 'legacy')
    capture_io { install_verified_binary(source, install_dir) }
    assert_equal Digest::SHA256.file(source).hexdigest, Digest::SHA256.file(File.join(install_dir, 'pod')).hexdigest
    assert_equal 0o755, File.stat(File.join(install_dir, 'pod')).mode & 0o777
    refute File.exist?(File.join(install_dir, 'abs'))
  end

  def test_install_detects_corrupted_copy_and_retains_legacy
    source = File.join(@fixture, 'pod')
    install_dir = File.join(@fixture, 'bin')
    fake_bin = File.join(@fixture, 'fake-bin')
    FileUtils.mkdir_p([install_dir, fake_bin])
    File.write(source, 'expected binary')
    legacy = File.join(install_dir, 'abs')
    File.write(legacy, 'legacy')
    body = "File.binwrite(ARGV.last, 'corrupted') unless ARGV.first == '-d'"
    write_executable(File.join(fake_bin, 'install'), body)
    script = "load #{File.join(__dir__, 'install').inspect}; install_verified_binary(#{source.inspect}, #{install_dir.inspect})"
    stdout, stderr, status = Open3.capture3({ 'PATH' => "#{fake_bin}:#{ENV.fetch('PATH')}" }, RbConfig.ruby, '-e', script)
    refute status.success?
    assert_includes stderr, 'Installation verification failed'
    refute_includes stdout, 'Installed and verified'
    assert_equal 'legacy', File.read(legacy)
  end
end
