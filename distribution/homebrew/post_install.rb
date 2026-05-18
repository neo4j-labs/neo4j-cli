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

# PostInstallHelper provides a testable wrapper around the Homebrew formula
# post_install block defined in .goreleaser.yaml. The logic here must stay
# in sync with the post_install: field in the brews: section of
# .goreleaser.yaml. GoReleaser wraps that string verbatim in a Ruby method,
# so only the method body is stored there; this module provides the same
# body as a callable method for testing purposes.
#
# Local test run:
#   ruby distribution/homebrew/tests/post_install_test.rb
module PostInstallHelper
  # post_install mirrors the body of the post_install method that GoReleaser
  # generates in the Homebrew formula. The `bin` accessor must be set to the
  # formula's bin path before calling this method (done automatically in a
  # real formula; supplied by the test harness in tests).
  def post_install
    return unless ENV["NEO4J_CLI_AUTO_INSTALL_SKILL"] == "1"
    system "#{bin}/neo4j-cli", "skill", "install", "--rw"
    true
  end
end
