/*
 * The MCP project's own client asks a server started as a program, and
 * writes down what it was told as one line of JSON. It judges nothing: the
 * Go test beside it does (official_test.go).
 *
 *   node ask.mjs <legacy|auto|pin> <program> [its arguments...]
 *
 * legacy is the handshake of the 2025 revisions, auto asks server/discover
 * first and falls back, pin accepts the 2026-07-28 revision and nothing
 * else. The server gets this process's environment, which the test chose.
 */
import { Client } from '@modelcontextprotocol/client';
import { StdioClientTransport } from '@modelcontextprotocol/client/stdio';

const [how, program, ...its] = process.argv.slice(2);
const mode = how === 'pin' ? { pin: '2026-07-28' } : how;

const told = { mode: how, faults: [], said: '' };
const why = (e) => ({ kind: e?.constructor?.name ?? typeof e, code: e?.code ?? null, message: String(e?.message ?? e) });

const transport = new StdioClientTransport({ command: program, args: its, env: { ...process.env }, stderr: 'pipe' });
transport.stderr?.on('data', (chunk) => { told.said += chunk; });

const client = new Client({ name: 'the official client', version: '2.4.0' }, { versionNegotiation: { mode } });
client.onerror = (e) => told.faults.push(why(e));

const attempt = async (what) => {
  try {
    return { answer: await what() };
  } catch (e) {
    return { refused: why(e) };
  }
};

const call = (name, args) => attempt(() => client.callTool({ name, arguments: args }));

try {
  await client.connect(transport);

  told.connected = true;
  told.pid = transport.pid ?? null;
  told.era = client.getProtocolEra() ?? null;
  told.version = client.getNegotiatedProtocolVersion() ?? null;
  told.server = client.getServerVersion() ?? null;
  told.capabilities = client.getServerCapabilities() ?? null;
  told.instructions = client.getInstructions() ?? null;

  told.ping = await attempt(() => client.ping());
  told.list = await attempt(() => client.listTools());
  told.again = await attempt(() => client.listTools());
  told.status = await call('assistant_status', {});
  told.found = await call('list_programs', { search: 'gimp' });
  told.narrowed = await call('list_programs', { kind: 'deb', limit: 1, include_removed: true });
  told.extra = await call('list_programs', { nonsense: true });
  told.wrong = await call('list_programs', { limit: 'many' });
  told.unknown = await call('no_such_tool', {});

  // A question to the assistant itself, and a search of what it remembers.
  told.asked = await call('ask_assistant', { question: 'What is the capital of Bulgaria?' });
  told.fetched = await call('ask_assistant', { answer_to: '0123456789abcdef0123456789abcdef' });
  told.both = await call('ask_assistant', { question: 'One?', answer_to: '0123456789abcdef0123456789abcdef' });
  told.remembered = await call('search_memory', { query: 'accountant', limit: 2 });
  told.nothingToFind = await call('search_memory', {});

  // A permission question, as Claude Code hands one to its permission
  // tool: answered with one block of text, a line of JSON.
  told.permitted = await call('permission_prompt', { tool_name: 'Bash', input: { command: 'ls -la' }, tool_use_id: 'toolu_01' });
  told.notPermitted = await call('permission_prompt', { tool_name: 'Write', input: { file_path: '/home/you/a.txt', content: 'x' }, tool_use_id: 'toolu_02' });

  // Three at once: each is answered once, to the one who asked.
  told.together = await Promise.all([call('assistant_status', {}), call('list_programs', { search: 'gimp' }), call('assistant_status', {})]);
} catch (e) {
  told.failed = why(e);
}

const before = Date.now();

await client.close().catch((e) => { told.closing = why(e); });

told.closedInMs = Date.now() - before;

if (told.pid) {
  try {
    process.kill(told.pid, 0);
    told.stillThere = true;
  } catch {
    told.stillThere = false;
  }
}

process.stdout.write(JSON.stringify(told) + '\n');
