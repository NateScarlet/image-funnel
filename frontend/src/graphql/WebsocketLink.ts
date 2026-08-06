import { ApolloLink, Observable } from "@apollo/client/core";
import type { Client, ClientOptions } from "graphql-ws";
import { createClient } from "graphql-ws";
import type { GraphQLError } from "graphql";
import { print } from "graphql";
import useNotification from "@/composables/useNotification";
import type OperationContext from "./OperationContext";

export default class WebSocketLink extends ApolloLink {
  static ERROR_CODE_RESTART = 4205;

  private client: Client;

  private restartFn: () => Promise<void>;

  private restartRequested: boolean;

  private onOpenedOnce: (() => void)[] = [];

  constructor(options: ClientOptions) {
    super();
    this.restartRequested = false;

    const { show, remove } = useNotification();
    let reconnectingNotifId: number | undefined;

    this.restartFn = async () => {
      this.restartRequested = true;
      return new Promise((resolve) => {
        this.onOpenedOnce.push(resolve);
      });
    };
    this.client = createClient({
      ...options,
      on: {
        ...options.on,
        opened: (socket) => {
          // 重连成功后清除重连通知
          if (reconnectingNotifId !== undefined) {
            remove(reconnectingNotifId);
            reconnectingNotifId = undefined;
          }
          while (this.onOpenedOnce.length) {
            this.onOpenedOnce.pop()?.();
          }
          const s = socket as unknown as WebSocket;
          this.restartFn = async () => {
            if (s.readyState !== WebSocket.OPEN) {
              this.restartRequested = true;
              return;
            }
            s.close(WebSocketLink.ERROR_CODE_RESTART, "Client Restart");
            return new Promise((resolve) => {
              this.onOpenedOnce.push(resolve);
            });
          };
          if (this.restartRequested) {
            this.restartRequested = false;
            void this.restartFn();
          }
          options.on?.opened?.(socket);
        },
        closed: () => {
          if (reconnectingNotifId === undefined) {
            reconnectingNotifId = show("正在重连…", "info", 0, undefined, undefined, true);
          }
        },
        error: () => {
          if (reconnectingNotifId === undefined) {
            reconnectingNotifId = show("正在重连…", "info", 0, undefined, undefined, true);
          }
        },
      },
    });
  }

  public readonly request = (operation: ApolloLink.Operation): Observable<ApolloLink.Result> => {
    const { includeQuery } = (operation.getContext() as OperationContext).http ?? {};
    return new Observable((observer) => {
      return this.client.subscribe(
        {
          ...operation,
          query: includeQuery !== false ? print(operation.query) : "",
        },
        {
          next: observer.next.bind(observer),
          complete: observer.complete.bind(observer),
          error: (err) => {
            if (Array.isArray(err)) {
              observer.next({ errors: err as unknown as GraphQLError[] });
              observer.complete();
            } else {
              observer.error(err);
            }
          },
        },
      );
    });
  };

  public readonly restart = async (): Promise<void> => {
    await this.restartFn();
  };
}
