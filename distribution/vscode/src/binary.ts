import * as fs from 'fs';
import * as path from 'path';
import * as vscode from 'vscode';

const SUPPORTED_PLATFORMS: Record<string, string> = {
  'darwin-arm64': 'neo4j-cli',
  'darwin-x64': 'neo4j-cli',
  'linux-arm64': 'neo4j-cli',
  'linux-x64': 'neo4j-cli',
  'win32-arm64': 'neo4j-cli.exe',
  'win32-x64': 'neo4j-cli.exe',
};

export function findBinary(context: vscode.ExtensionContext): string {
  // 1. User-configured override
  const configured = vscode.workspace.getConfiguration('neo4j-cli').get<string>('binaryPath');
  if (configured && configured.trim() !== '') {
    return configured.trim();
  }

  // 2. Bundled binary (present in platform-specific .vsix)
  const exeName = process.platform === 'win32' ? 'neo4j-cli.exe' : 'neo4j-cli';
  const bundled = path.join(context.extensionPath, 'bin', exeName);
  if (fs.existsSync(bundled)) {
    return bundled;
  }

  // 3. System PATH fallback (useful in development)
  const key = `${process.platform}-${process.arch}`;
  if (!SUPPORTED_PLATFORMS[key]) {
    vscode.window.showWarningMessage(
      `Neo4j CLI: no bundled binary for platform "${key}". Install neo4j-cli manually and set neo4j-cli.binaryPath in settings.`
    );
  }
  return 'neo4j-cli';
}
