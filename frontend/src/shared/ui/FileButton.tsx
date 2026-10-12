import { useRef, type ReactNode } from 'react';
import { Button, type ButtonProps } from './Button';

export interface FileButtonProps extends Omit<ButtonProps, 'onClick' | 'onChange'> {
  accept?: string;
  onFile: (file: File) => void;
  children: ReactNode;
}

/** Button that opens the native file picker. */
export function FileButton({ accept, onFile, children, ...rest }: FileButtonProps) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={input}
        type="file"
        accept={accept}
        hidden
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) onFile(file);
          e.target.value = '';
        }}
      />
      <Button {...rest} onClick={() => input.current?.click()}>
        {children}
      </Button>
    </>
  );
}
