/**
 * Automatic JSX runtime that renders through the Kandev host's React, so the
 * bundle never ships its own React.
 */
import { currentHost } from "../runtime";

type Props = Record<string, unknown> & { children?: unknown };

export function jsx(type: unknown, props: Props | null, key?: unknown): unknown {
  const { children, ...rest } = props ?? {};
  const finalProps = key === undefined ? rest : { ...rest, key };
  if (children === undefined) return currentHost().jsx(type, finalProps);
  if (Array.isArray(children)) return currentHost().jsx(type, finalProps, ...children);
  return currentHost().jsx(type, finalProps, children);
}

export const jsxs = jsx;

/** Fragment groups children without a wrapper element. */
export function Fragment(props: { children?: unknown }): unknown {
  return currentHost().jsx(currentHost().React.Fragment, {}, props.children);
}

// eslint-disable-next-line @typescript-eslint/no-namespace -- the JSX runtime contract requires this namespace.
export namespace JSX {
  export type Element = unknown;
  export type ElementType = unknown;
  export interface ElementChildrenAttribute {
    children: unknown;
  }
  export interface IntrinsicAttributes {
    key?: string | number | null;
  }
  type Handler<E> = (event: E) => void;
  export interface IntrinsicProps {
    [prop: string]: unknown;
    onClick?: Handler<MouseEvent>;
    onKeyDown?: Handler<KeyboardEvent>;
    onChange?: Handler<Event & { target: HTMLInputElement }>;
    onDragStart?: Handler<DragEvent>;
    onDragEnd?: Handler<DragEvent>;
    onDragOver?: Handler<DragEvent>;
    onDragEnter?: Handler<DragEvent>;
    onDragLeave?: Handler<DragEvent>;
    onDrop?: Handler<DragEvent>;
    onPointerDown?: Handler<PointerEvent>;
    onFocus?: Handler<FocusEvent>;
    onBlur?: Handler<FocusEvent>;
  }
  export interface IntrinsicElements {
    [element: string]: IntrinsicProps;
  }
}
