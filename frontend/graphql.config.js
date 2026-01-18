export default {
  schema: [
    "../graph/scalars.graphql",
    "../graph/directives.graphql",
    "../graph/types/*.graphql",
    "../graph/enums/*.graphql",
    "../graph/queries/*.graphql",
    "../graph/subscriptions/*.graphql",
    "../graph/mutations/*.graphql"
  ],
  documents: ["src/graphql/**/*.gql"],
}
