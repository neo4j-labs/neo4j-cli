import { spawnSync } from 'child_process';

export interface CliResult {
  ok: boolean;
  stdout: string;
  stderr: string;
  status: number;
}

export function run(binary: string, args: string[]): CliResult {
  const result = spawnSync(binary, args, { encoding: 'utf8', maxBuffer: 10 * 1024 * 1024 });
  return {
    ok: result.status === 0,
    stdout: result.stdout ?? '',
    stderr: result.stderr ?? '',
    status: result.status ?? 1,
  };
}

export function runJSON<T>(binary: string, args: string[]): T | null {
  const r = run(binary, [...args, '--format', 'json']);
  if (!r.ok) return null;
  try {
    return JSON.parse(r.stdout) as T;
  } catch {
    return null;
  }
}

// Unwraps Aura API responses that nest their payload under a "data" key.
export function unwrapData<T>(value: unknown): T[] {
  if (!value) return [];
  if (Array.isArray(value)) return value as T[];
  const obj = value as Record<string, unknown>;
  if (Array.isArray(obj['data'])) return obj['data'] as T[];
  return [];
}
