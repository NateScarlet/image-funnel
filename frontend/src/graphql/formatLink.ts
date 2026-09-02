import { ApolloLink } from "@apollo/client/core";
import { getMainDefinition } from "@apollo/client/utilities";
import { Kind, OperationTypeNode } from "graphql";
import { ImageFormat } from "@/graphql/generated";
import isImageFormatSupported from "@/utils/image-format";

// #region 格式注入 Link
/**
 * 在所有包含 format 变量的查询/订阅中注入客户端支持的最优格式。
 * 格式探测结果仅执行一次并缓存，后续操作直接读取缓存值。
 * 不含 format 变量的操作不受影响。
 */
const formatLink = new ApolloLink((operation, forward) => {
  const definition = getMainDefinition(operation.query);

  if (
    definition.kind === Kind.OPERATION_DEFINITION &&
    (definition.operation === OperationTypeNode.QUERY ||
      definition.operation === OperationTypeNode.SUBSCRIPTION)
  ) {
    // 检查该操作是否定义了 format 变量
    const hasFormatVar = definition.variableDefinitions?.some(
      (v) => v.variable.name.value === "format",
    );

    if (hasFormatVar) {
      const avifSupport = isImageFormatSupported("image/avif");
      const format = avifSupport.value === true ? ImageFormat.AVIF : ImageFormat.WEBP;
      operation.variables = {
        ...operation.variables,
        format,
      };
    }
  }

  return forward(operation);
});
// #endregion

export default formatLink;
