import type { PolicyLogsRepository } from '$lib/repos/repository';

const testData = [
  '\x1B[30;43m\x1B[30;43m WARNING \x1B[0m\x1B[0m \x1B[33m\x1B[33mSpawni…ne for pod pepr-uds-core-74c5d4dd76-cklgr\x1B[0m\x1B[0m\n',
  '\x1B[30;43m\x1B[30;43m WARNING \x1B[0m\x1B[0m \x1B[33m\x1B[33mSpawni…od pepr-uds-core-watcher-56d9c84c95-xbnt2\x1B[0m\x1B[0m\n',
  '\x1B[30;43m\x1B[30;43m WARNING \x1B[0m\x1B[0m \x1B[33m\x1B[33mSpawni…ne for pod pepr-uds-core-74c5d4dd76-phrcd\x1B[0m\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;135m ⚙️ OPERATOR  keycloak/keycloak\x1B[0m\x1B[0m\x1B[38;5;102m\x1B[0m\n',
  '\x1B[38;5;102m                                  Processing Package keycloak/keycloak\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;32m ✎ MUTATED   keycloak/keycloak-headless\x1B[0m\x1B[0m\x1B[1m\x1B[0m\n',
  '\x1B[1m                        ADDED:\x1B[0m\n',
  '\x1B[38;5;102m/metadata/annotations/uds-core.pepr.dev~1uds-core-policies\x1B[0m=\x1B[38;5;32m"succeeded"\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;135m ⚙️ OPERATOR  keycloak/keycloak\x1B[0m\x1B[0m\x1B[38;5;102m\x1B[0m\n',
  '\x1B[38;5;102m                                  Updating status to Pending\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;135m ⚙️ OPERATOR  keycloak/keycloak\x1B[0m\x1B[0m\x1B[38;5;102m\x1B[0m\n',
  '\x1B[38;5;102m                                  Updating status to Ready\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;135m ⚙️ OPERATOR  keycloak/keycloak\x1B[0m\x1B[0m\x1B[38;5;102m\x1B[0m\n',
  '\x1B[38;5;102m                                  Processing Package keycloak/keycloak\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;32m ✎ MUTATED   keycloak/keycloak-http\x1B[0m\x1B[0m\x1B[1m\x1B[0m\n',
  '\x1B[1m                        ADDED:\x1B[0m\n',
  '\x1B[38;5;102m/metadata/annotations/uds-core.pepr.dev~1uds-core-policies\x1B[0m=\x1B[38;5;32m"succeeded"\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;135m ⚙️ OPERATOR  keycloak/keycloak\x1B[0m\x1B[0m\x1B[38;5;102m\x1B[0m\n',
  '\x1B[38;5;102m                                  Processing Package keycloak/keycloak\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;35m ✓ ALLOWED   ke…oak-http\x1B[0m\x1B[0m \x1B[38;5;102m(repeated 1 time)\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;35m ✓ ALLOWED   ke…headless\x1B[0m\x1B[0m \x1B[38;5;102m(repeated 1 time)\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;35m ✓ ALLOWED   keycloak/keycloak\x1B[0m\x1B[0m\n',
  '2024-06-06 11:06:06  \x1B[1m\x1B[38;5;32m ✎ MUTATED   keycloak/keycloak-0\x1B[0m\x1B[0m\x1B[1m\x1B[0m\n',
  '\x1B[1m                        ADDED:\x1B[0m\n',
  '\x1B[38;5;102m/spec/securityContext/runAsNonRoot\x1B[0m=\x1B[38;5;32mtrue\x1B[0m\n',
  '\x1B[38;5;102m/spec/securityContext/runAsUser\x1B[0m=\x1B[38;5;32m1000\x1B[0m\n',
  '\x1B[38;5;102m/spec/securityContext/runAsGroup\x1B[0m=\x1B[38;5;32m1000\x1B[0m\n'
];

export class FakePolicyLogsRepo implements PolicyLogsRepository {
  eventSource: {
    onmessage: (message: string) => void;
    onerror: (error: Event) => void;
    close: () => void;
  };

  messageHandler: (message: string) => void;
  errorHandler: (error: Event) => void;

  constructor(src?: string) {
    this.eventSource = {
      onmessage: () => {},
      onerror: () => {},
      close: () => {}
    };
    this.messageHandler = () => {};
    this.errorHandler = () => {};
  }

  onMessageHandler(handler: (message: string) => void) {
    this.messageHandler = handler;
    this.eventSource.onmessage = (message) => {
      this.messageHandler(message);
    };

    const interval = setInterval(() => {
      this.eventSource.onmessage(testData.toString().replace(/,/g, ''));
    }, 1000);

    // stop the interval after 15 seconds
    setTimeout(() => {
      clearInterval(interval);
    }, 1000 * 5);
  }

  onErrorHandler(handler: (error: Event) => void) {
    this.errorHandler = handler;
    this.eventSource.onerror = (error) => {
      this.errorHandler(error);
    };
  }

  close() {
    this.eventSource.close();
    console.log('Connection to server closed.');
  }
}
