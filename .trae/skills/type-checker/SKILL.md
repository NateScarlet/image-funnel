---
name: "type-checker"
description: "Runs GraphQL type generation and TypeScript type checking. Invoke when user wants to check for type errors or validate GraphQL queries/mutations."
---

# Type Checker

This skill helps you check for type errors in the frontend codebase by running two commands:

## Commands

### 1. Generate GraphQL Types
```bash
pnpm run generate:graphql
```
This command generates TypeScript types from GraphQL schema and queries. It validates that all GraphQL queries and mutations match the backend schema.

### 2. TypeScript Type Check
```bash
pnpm run build:tsc
```
This command runs vue-tsc to check all TypeScript types in the frontend code, including Vue components.

## When to Use

Invoke this skill when:
- User asks to check for type errors
- User wants to validate GraphQL queries/mutations
- After modifying GraphQL queries or mutations
- After updating the GraphQL schema
- Before committing changes to ensure type safety
- User reports type-related errors

## Workflow

1. First, ensure the backend server is running on `http://localhost:8080` for GraphQL schema introspection
2. Run `pnpm run generate:graphql` to generate types from the schema
3. Run `pnpm run build:tsc` to check all TypeScript types
4. Review and fix any errors reported

## Common Issues

- **GraphQL Schema Connection**: Ensure backend is running before running generate:graphql
- **Type Mismatches**: Check that frontend queries match the backend schema
- **Missing Dependencies**: Ensure all required packages are installed
- **Vue Component Types**: Make sure `.vue` files are properly typed
