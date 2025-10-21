import {
  Panel,
  PanelBody,
  PanelFooter,
  PanelHeader,
} from '@/components/ui/panel'

export default function Dashboard() {
  return (
    <Panel>
      <PanelHeader>Header</PanelHeader>
      <PanelBody>Body</PanelBody>
      <PanelFooter>Footer</PanelFooter>
    </Panel>
  )
}
