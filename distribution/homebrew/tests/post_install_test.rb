# Copyright (c) "Neo4j"
# Neo4j Sweden AB [https://neo4j.com]
# This file is part of Neo4j.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Behavioral tests for the Homebrew formula post_install block.
# Mirrors the post_install: body from the brews: section in .goreleaser.yaml.
#
# Local run (no Homebrew or neo4j-cli required):
#   ruby distribution/homebrew/tests/post_install_test.rb

require "minitest/autorun"
require_relative "../post_install"

# FormulaStub simulates the Homebrew Formula DSL context in which the
# post_install method runs. It includes PostInstallHelper (providing the
# post_install method body), exposes a configurable `bin` path, and overrides
# `system` so tests can record or raise on calls without invoking real processes.
class FormulaStub
  include PostInstallHelper

  attr_accessor :bin, :system_calls, :system_return

  def initialize(bin_path: "/usr/local/bin")
    @bin = bin_path
    @system_calls = []
    @system_return = true
  end

  # Override Kernel#system to record invocations and return a configurable value.
  # Homebrew's system DSL returns false on failure — it never raises.
  def system(*args)
    @system_calls << args
    @system_return
  end
end

class PostInstallTest < Minitest::Test
  def setup
    # Ensure env var is clean before each test.
    ENV.delete("NEO4J_CLI_AUTO_INSTALL_SKILL")
  end

  def teardown
    ENV.delete("NEO4J_CLI_AUTO_INSTALL_SKILL")
  end

  # Test 1: env=1 — system is called with the correct binary path and args.
  def test_env_set_to_1_calls_skill_install
    ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] = "1"
    formula = FormulaStub.new(bin_path: "/opt/homebrew/bin")

    formula.post_install

    assert_equal 1, formula.system_calls.size, "Expected system to be called once"
    assert_equal ["/opt/homebrew/bin/neo4j-cli", "skill", "install", "--rw"],
                 formula.system_calls.first,
                 "Expected system call with correct binary path and skill args"
  end

  # Test 2a: env unset — system is never called.
  def test_env_unset_does_not_call_system
    # ENV var already deleted in setup.
    formula = FormulaStub.new

    formula.post_install

    assert_empty formula.system_calls, "Expected system not to be called when env var is unset"
  end

  # Test 2b: env=0 — system is never called.
  def test_env_set_to_0_does_not_call_system
    ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] = "0"
    formula = FormulaStub.new

    formula.post_install

    assert_empty formula.system_calls, "Expected system not to be called when env var is '0'"
  end

  # Test 3: env=1 but system returns false (command failed) — post_install still returns true.
  # Homebrew's system DSL returns false on failure; it does not raise. The formula
  # must not propagate the failure to brew (which would abort the install).
  def test_env_set_to_1_system_returns_false_still_returns_true
    ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] = "1"
    formula = FormulaStub.new
    formula.system_return = false

    result = formula.post_install

    assert_equal 1, formula.system_calls.size, "Expected system to be called once"
    assert_equal true, result, "Expected post_install to return true even when system fails"
  end
end
