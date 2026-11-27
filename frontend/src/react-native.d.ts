declare module 'react-native' {
  import type { CSSProperties, FC, ReactNode } from 'react';
  type NativePressableProps = {
    children?: ReactNode;
    onPress?: () => void;
    style?: CSSProperties | ((state: { pressed: boolean }) => CSSProperties);
    accessibilityLabel?: string;
  };
  export const View: FC<{ children?: ReactNode; style?: CSSProperties; accessibilityLabel?: string }>;
  export const Text: FC<{ children?: ReactNode; style?: CSSProperties }>;
  export const Pressable: FC<NativePressableProps>;
}
