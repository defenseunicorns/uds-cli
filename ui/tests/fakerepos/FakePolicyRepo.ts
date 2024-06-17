import type {PolicyLogsRepository} from "$lib/repos/repository";

const testData= '✅  VALIDATE   keycloak/keycloak-0 (e1fdc5b1-0a56-46df-9931-2baa7005a7aa)';

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
    }
    this.messageHandler = () => {};
    this.errorHandler = () => {};
  }

  onMessageHandler(handler: (message: string) => void) {
    this.messageHandler = handler;
    this.eventSource.onmessage = (message) => {
      this.messageHandler(message);
    };

    const interval = setInterval(() => {
        this.eventSource.onmessage(testData);
    }, 10)

    // stop the interval after 15 seconds
    setTimeout(() => {
      clearInterval(interval)
    }, 1000 * 2)
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
