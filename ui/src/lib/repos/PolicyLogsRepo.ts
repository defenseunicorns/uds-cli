import type { PolicyLogsRepository } from '$lib/repos/repository';

export class PolicyLogsRepo implements PolicyLogsRepository {
  eventSource: EventSource;

  constructor(src: string) {
    this.eventSource = new EventSource(src);
  }

  onMessageHandler(handler: (message: string) => void) {
    this.eventSource.onmessage = (message) => {
      handler(message.data);
    };
  }

  onErrorHandler(handler: (error: Event) => void) {
    this.eventSource.onerror = (error) => {
      handler(error);
    };
  }

  close() {
    this.eventSource.close();
  }
}
