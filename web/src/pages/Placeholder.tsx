import { Result, Button } from 'antd'
import { useNavigate } from 'react-router-dom'

interface Props {
  title: string
  phase?: string
}

export default function Placeholder({ title, phase }: Props) {
  const navigate = useNavigate()
  return (
    <Result
      status="info"
      title={title}
      subTitle={`该页面正在开发中${phase ? `（${phase}）` : ''}，敬请期待。`}
      extra={
        <Button type="primary" onClick={() => navigate('/')}>
          返回首页
        </Button>
      }
    />
  )
}
