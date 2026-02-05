import { ApolloLink, Observable } from "@apollo/client/core";
import type { Client, ClientOptions } from "graphql-ws";
import { createClient } from "graphql-ws";
import type { GraphQLError } from "graphql";
import { print } from "graphql";
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
          while (this.onOpenedOnce.length) {
            this.onOpenedOnce.pop()?.();
          }
          const s = socket as WebSocket;
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
            this.restartFn();
          }
          options.on?.opened?.(socket);
        },
      },
    });
  }

  public readonly request = (
    operation: ApolloLink.Operation,
  ): Observable<ApolloLink.Result> => {
    const { includeQuery } =
      (operation.getContext() as OperationContext).http ?? {};
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
              observer.next({ errors: err as GraphQLError[] });
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
