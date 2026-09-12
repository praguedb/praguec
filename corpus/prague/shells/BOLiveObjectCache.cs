namespace BO.Services.Prague.Models;

public sealed partial class BOLiveObjectCache
{
    public void Clear()
    {
        foreach (var item in Query().Execute())
        {
            Remove(item.EventId);
        }
    }
}