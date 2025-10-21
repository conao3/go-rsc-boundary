"use client"
import * as React from 'react'

export function Panel({ children }: React.PropsWithChildren) {
  return <section>{children}</section>
}

export function PanelHeader({ children }: React.PropsWithChildren) {
  return <header>{children}</header>
}

export function PanelBody({ children }: React.PropsWithChildren) {
  return <main>{children}</main>
}

export function PanelFooter({ children }: React.PropsWithChildren) {
  return <footer>{children}</footer>
}
