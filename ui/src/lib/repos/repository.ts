export interface PolicyLogsRepository {
  onMessageHandler(handler: (message: string) => void): void;
  onErrorHandler(handler: (error: Event) => void): void;
  close(): void;
}
