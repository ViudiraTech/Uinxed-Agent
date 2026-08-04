/*
 * Copyright 2026 Uinxed Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 */

import React, { useEffect, useState } from "react";
import { Text, useInput } from "ink";

/* ink-text-input 会把 Ctrl+T/O/E 等组合键中的字母写进输入框。
 * 这个受控输入框会过滤全部 Ctrl/Meta 组合，同时支持禁用、粘贴和光标移动。 */
export default function SafeTextInput({
  value = "",
  placeholder = "",
  focus = true,
  disabled = false,
  onChange,
  onSubmit,
}) {
  const [cursor, setCursor] = useState(value.length);

  useEffect(() => {
    setCursor((n) => Math.min(n, value.length));
  }, [value]);

  useInput((input, key) => {
    if (key.ctrl || key.meta || key.escape || key.tab || key.upArrow || key.downArrow || key.pageUp || key.pageDown) return;
    if (key.return) {
      onSubmit?.(value);
      return;
    }
    if (key.leftArrow) {
      setCursor((n) => Math.max(0, n - 1));
      return;
    }
    if (key.rightArrow) {
      setCursor((n) => Math.min(value.length, n + 1));
      return;
    }
    if (key.backspace || key.delete) {
      if (cursor === 0) return;
      onChange?.(value.slice(0, cursor - 1) + value.slice(cursor));
      setCursor((n) => n - 1);
      return;
    }
    if (!input) return;
    onChange?.(value.slice(0, cursor) + input + value.slice(cursor));
    setCursor((n) => n + input.length);
  }, { isActive: focus && !disabled });

  if ((!focus || disabled) && !value) return <Text dimColor>{placeholder}</Text>;
  if (!focus || disabled) return <Text>{value}</Text>;
  if (!value) {
    return (
      <Text>
        <Text inverse>{placeholder ? placeholder[0] : " "}</Text>
        <Text dimColor>{placeholder.slice(1)}</Text>
      </Text>
    );
  }

  return (
    <Text>
      {value.slice(0, cursor)}
      <Text inverse>{cursor < value.length ? value[cursor] : " "}</Text>
      {value.slice(cursor + (cursor < value.length ? 1 : 0))}
    </Text>
  );
}
