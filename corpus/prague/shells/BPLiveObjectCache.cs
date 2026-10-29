namespace Common.EventApi.Cache.Prague.Models;

public sealed partial class BPLiveObjectCache
{
    public void Clear()
    {
        foreach (var item in Query().Execute())
        {
            Remove(item.EventId);
        }
    }
}