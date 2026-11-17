import { spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { createServer, connect as connectSocket } from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";

/**
 * Loopback JetStream server for shared-session tests. It uses the repository's bin/nats-server (as the Go integration
 * tests do) or WASL_NATS_SERVER. Tests skip when no binary is present unless WASL_REQUIRE_NATS_TESTS=1, which CI sets.
 */
export function natsServerBinary(): string | undefined {
  const candidates = [process.env.WASL_NATS_SERVER, path.resolve(process.cwd(), "..", "bin", "nats-server"), path.resolve(process.cwd(), "bin", "nats-server")];
  const found = candidates.find((candidate): candidate is string => Boolean(candidate) && existsSync(candidate!));
  if (!found && process.env.WASL_REQUIRE_NATS_TESTS === "1") throw new Error("WASL_REQUIRE_NATS_TESTS=1 but no nats-server binary was found (bin/nats-server or WASL_NATS_SERVER)");
  return found;
}

export async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => (typeof address === "object" && address ? resolve(address.port) : reject(new Error("no port"))));
    });
  });
}

async function listening(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = connectSocket(port, "127.0.0.1");
    socket.once("connect", () => {
      socket.destroy();
      resolve(true);
    });
    socket.once("error", () => resolve(false));
  });
}

export type TestNatsServer = {
  url: string;
  port: number;
  storeDir: string;
  /** Stops the process and starts it again on the same port and JetStream store directory. */
  restart(): Promise<void>;
  stop(): Promise<void>;
  close(): Promise<void>;
};

export async function startNatsServer(binary: string): Promise<TestNatsServer> {
  const port = await freePort();
  const storeDir = mkdtempSync(path.join(tmpdir(), "wasl-oidc-nats-"));
  let child: ChildProcess | undefined;

  const start = async () => {
    child = spawn(binary, ["-a", "127.0.0.1", "-p", String(port), "-js", "-sd", storeDir, "--server_name", "wasl-oidc-test"], { stdio: "ignore" });
    for (let attempt = 0; attempt < 100; attempt++) {
      if (await listening(port)) return;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    throw new Error("nats-server did not start");
  };
  const stop = async () => {
    const current = child;
    child = undefined;
    if (!current || current.exitCode !== null) return;
    const exited = new Promise((resolve) => current.once("exit", resolve));
    current.kill("SIGTERM");
    await exited;
  };

  await start();
  return {
    url: `nats://127.0.0.1:${port}`,
    port,
    storeDir,
    restart: async () => {
      await stop();
      await start();
    },
    stop,
    close: async () => {
      await stop();
      rmSync(storeDir, { recursive: true, force: true });
    },
  };
}
